package goast

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"sort"
	"strings"

	"github.com/fluxplane/codegate/internal/core"
)

type architecturePolicyAssessment struct {
	findings   []Finding
	violations []Violation
}

type architecturePackage struct {
	unit       string
	importPath string
	layer      string
	files      []string
}

func goArchitecturePolicyAssessment(ctx context.Context, snapshot Snapshot, idx *index, opts AssessmentOptions) (architecturePolicyAssessment, error) {
	if !assessmentGateEnabled(opts, AssessmentGateArchitecture) || opts.Architecture == nil {
		return architecturePolicyAssessment{}, nil
	}
	modulePath, err := architectureModulePath(ctx, snapshot, opts.Architecture)
	if err != nil {
		return architecturePolicyAssessment{}, err
	}
	effectiveRules := *opts.Architecture
	effectiveRules.ModulePath = modulePath
	rules := &effectiveRules
	packages := architecturePackages(idx, modulePath, rules)
	var out architecturePolicyAssessment
	out.violations = append(out.violations, architectureDependencyViolations(idx, packages, rules)...)
	out.violations = append(out.violations, architectureUnknownPackageViolations(packages, rules)...)
	out.violations = append(out.violations, architectureEffectImportViolations(idx, packages, rules)...)
	callViolations, err := architectureEffectCallViolations(ctx, snapshot, idx, packages, rules)
	if err != nil {
		return architecturePolicyAssessment{}, err
	}
	out.violations = append(out.violations, callViolations...)
	out.findings = append(out.findings, architectureCouplingFindings(idx, packages, rules)...)
	out.findings, out.violations = architectureDemoteWarningViolations(out.findings, out.violations)
	return out, nil
}

func architectureModulePath(ctx context.Context, snapshot Snapshot, rules *ArchitectureRules) (string, error) {
	if rules.ModulePath != "" {
		return rules.ModulePath, nil
	}
	return readModulePath(ctx, snapshot)
}

func architecturePackages(idx *index, modulePath string, rules *ArchitectureRules) map[string]architecturePackage {
	out := map[string]architecturePackage{}
	units := make([]string, 0, len(idx.unitFiles))
	for unit := range idx.unitFiles {
		units = append(units, unit)
	}
	sort.Strings(units)
	for _, unit := range units {
		files := append([]string(nil), idx.unitFiles[unit]...)
		sort.Strings(files)
		dir := packageDir(unit)
		if len(files) > 0 {
			dir = dirFromFilePath(files[0])
		}
		importPath := packageImportPath(dir, modulePath)
		out[unit] = architecturePackage{
			unit:       unit,
			importPath: importPath,
			layer:      architectureLayerForPackage(importPath, modulePath, rules),
			files:      files,
		}
	}
	return out
}

func architectureDependencyViolations(idx *index, packages map[string]architecturePackage, rules *ArchitectureRules) []Violation {
	if len(rules.Dependencies) == 0 {
		return nil
	}
	var out []Violation
	for _, imp := range idx.imports {
		from := packages[imp.FromUnit]
		toLayer := architectureLayerForPackage(imp.Import, rules.ModulePath, rules)
		if from.layer == "" || toLayer == "" {
			continue
		}
		allowed, reason := architectureDependencyAllowed(from.layer, toLayer, rules.Dependencies)
		if allowed || architectureExceptionMatches(rules.Exceptions, architectureBoundaryKind(architectureIsTestImport(imp)), from.importPath, imp.Import, "", from.layer, toLayer, rules.ModulePath) {
			continue
		}
		if reason == "" {
			reason = fmt.Sprintf("%s may not import %s", from.layer, toLayer)
		}
		out = append(out, Violation{
			Kind:     architectureBoundaryKind(architectureIsTestImport(imp)),
			Severity: architectureBoundarySeverity(architectureIsTestImport(imp)),
			Package:  from.importPath,
			Location: imp.Location,
			Reason:   reason,
		})
	}
	return out
}

func architectureUnknownPackageViolations(packages map[string]architecturePackage, rules *ArchitectureRules) []Violation {
	if len(rules.Layers) == 0 || rules.ModulePath == "" {
		return nil
	}
	var out []Violation
	for _, pkg := range packages {
		if pkg.layer != "" || !architectureInModule(pkg.importPath, rules.ModulePath) {
			continue
		}
		if architectureExceptionMatches(rules.Exceptions, "architecture_unknown_package", pkg.importPath, "", "", "", "", rules.ModulePath) {
			continue
		}
		out = append(out, Violation{
			Kind:     "architecture_unknown_package",
			Severity: "error",
			Package:  pkg.importPath,
			Reason:   "Package is inside the module but does not match any configured architecture layer.",
		})
	}
	return out
}

func architectureEffectImportViolations(idx *index, packages map[string]architecturePackage, rules *ArchitectureRules) []Violation {
	var out []Violation
	for _, rule := range rules.Effects {
		if len(rule.Imports) == 0 {
			continue
		}
		for _, imp := range idx.imports {
			pkg := packages[imp.FromUnit]
			if !architectureScopeMatches(rule.Scope, pkg, imp.FromPath, rules.ModulePath) || !architectureAnyPatternMatches(rule.Imports, imp.Import, rules.ModulePath) {
				continue
			}
			kind := architectureEffectKind(rule, "architecture_effect_import")
			if architectureEffectAllowed(rule) || architectureExceptionMatches(rules.Exceptions, kind, pkg.importPath, imp.Import, "", pkg.layer, "", rules.ModulePath) {
				continue
			}
			out = append(out, Violation{
				Kind:     kind,
				Severity: architectureEffectSeverity(rule),
				Package:  pkg.importPath,
				Location: imp.Location,
				Reason:   architectureEffectReason(rule, fmt.Sprintf("Import %q is denied by architecture effect rule.", imp.Import)),
			})
		}
	}
	return out
}

func architectureEffectCallViolations(ctx context.Context, snapshot Snapshot, idx *index, packages map[string]architecturePackage, rules *ArchitectureRules) ([]Violation, error) {
	var out []Violation
	for _, doc := range idx.documents {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		pkg := packages[doc.UnitID]
		src, err := snapshot.ReadFile(ctx, doc.URI)
		if err != nil {
			return nil, err
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, doc.URI, src, 0)
		if err != nil {
			continue
		}
		aliases := architectureImportAliases(file)
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			importPath := aliases[ident.Name]
			if importPath == "" {
				return true
			}
			for _, rule := range rules.Effects {
				if len(rule.Calls) == 0 || !architectureScopeMatches(rule.Scope, pkg, doc.URI, rules.ModulePath) {
					continue
				}
				if !architectureCallMatches(rule.Calls, importPath, selector.Sel.Name, rules.ModulePath) {
					continue
				}
				kind := architectureEffectKind(rule, "architecture_effect_call")
				symbol := importPath + "." + selector.Sel.Name
				if architectureEffectAllowed(rule) || architectureExceptionMatches(rules.Exceptions, kind, pkg.importPath, importPath, symbol, pkg.layer, "", rules.ModulePath) {
					continue
				}
				out = append(out, Violation{
					Kind:     kind,
					Severity: architectureEffectSeverity(rule),
					Package:  pkg.importPath,
					Symbol:   symbol,
					Location: Location{URI: doc.URI, Range: rangeOf(fset, selector.Pos(), selector.End())},
					Reason:   architectureEffectReason(rule, fmt.Sprintf("Call %q is denied by architecture effect rule.", symbol)),
				})
			}
			return true
		})
	}
	return out, nil
}

func architectureCouplingFindings(idx *index, packages map[string]architecturePackage, rules *ArchitectureRules) []Finding {
	threshold := rules.Coupling.FanOutThreshold
	if threshold <= 0 {
		return nil
	}
	fanOut := map[string]int{}
	for _, imp := range idx.imports {
		pkg := packages[imp.FromUnit]
		if pkg.importPath == "" || architectureLayerForPackage(imp.Import, rules.ModulePath, rules) == "" {
			continue
		}
		fanOut[pkg.importPath]++
	}
	var out []Finding
	for _, pkg := range packages {
		if len(rules.Coupling.Layers) > 0 && !architectureStringIn(rules.Coupling.Layers, pkg.layer) {
			continue
		}
		count := fanOut[pkg.importPath]
		if count <= threshold {
			continue
		}
		reason, reviewed := architectureReviewedFanOutReason(rules.Coupling.ReviewedFanOut, pkg.importPath, rules.ModulePath)
		if reason == "" {
			reason = "Fan-out exceeds the configured architecture coupling threshold."
		}
		out = append(out, Finding{
			Kind:     "architecture_fan_out",
			Severity: "warning",
			Package:  pkg.importPath,
			Allowed:  reviewed,
			Reason:   reason,
		})
	}
	return out
}

func architectureDemoteWarningViolations(findings []Finding, violations []Violation) ([]Finding, []Violation) {
	kept := violations[:0]
	for _, violation := range violations {
		if violation.Severity == "error" {
			kept = append(kept, violation)
			continue
		}
		findings = append(findings, Finding{
			Kind:     violation.Kind,
			Severity: violation.Severity,
			Package:  violation.Package,
			Symbol:   violation.Symbol,
			Location: violation.Location,
			Reason:   violation.Reason,
		})
	}
	return findings, kept
}

func architectureLayerForPackage(importPath, modulePath string, rules *ArchitectureRules) string {
	for _, layer := range rules.Layers {
		for _, prefix := range layer.Prefixes {
			if architecturePackagePatternMatches(prefix, importPath, modulePath) {
				return layer.Name
			}
		}
	}
	return ""
}

func architectureDependencyAllowed(from, to string, rules []ArchitectureDependencyRule) (bool, string) {
	for _, rule := range rules {
		if !architectureLayerRuleMatches(rule.FromLayer, from) || !architectureLayerRuleMatches(rule.ToLayer, to) {
			continue
		}
		action := rule.Action
		if action == "" {
			action = ArchitectureRuleAllow
		}
		return action == ArchitectureRuleAllow, rule.Reason
	}
	return false, ""
}

func architectureScopeMatches(scope ArchitectureScope, pkg architecturePackage, filePath, modulePath string) bool {
	if len(scope.Layers) == 0 && len(scope.Packages) == 0 && len(scope.Paths) == 0 {
		return true
	}
	if architectureStringIn(scope.Layers, pkg.layer) {
		return true
	}
	for _, pattern := range scope.Packages {
		if architecturePatternMatches(pattern, pkg.importPath, modulePath) {
			return true
		}
	}
	for _, pattern := range scope.Paths {
		if architecturePatternMatches(pattern, core.CleanPath(filePath), "") {
			return true
		}
	}
	return false
}

func architectureCallMatches(rules []ArchitectureCallRule, importPath, symbol, modulePath string) bool {
	for _, rule := range rules {
		if architecturePatternMatches(rule.Import, importPath, modulePath) && (rule.Symbol == "" || rule.Symbol == symbol) {
			return true
		}
	}
	return false
}

func architectureAnyPatternMatches(patterns []string, value, modulePath string) bool {
	for _, pattern := range patterns {
		if architecturePatternMatches(pattern, value, modulePath) {
			return true
		}
	}
	return false
}

func architecturePatternMatches(pattern, value, modulePath string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	candidates := []string{value}
	if modulePath != "" {
		candidates = append(candidates, strings.TrimPrefix(value, modulePath+"/"))
		if value == modulePath {
			candidates = append(candidates, ".")
		}
	}
	for _, candidate := range candidates {
		if candidate == pattern || strings.HasPrefix(candidate, strings.TrimSuffix(pattern, "/")+"/") {
			return true
		}
	}
	return false
}

func architecturePackagePatternMatches(pattern, importPath, modulePath string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	if modulePath == "" {
		return architecturePatternMatches(pattern, importPath, "")
	}
	if strings.HasPrefix(pattern, modulePath+"/") {
		return architecturePatternMatches(pattern, importPath, modulePath)
	}
	if importPath != modulePath && !strings.HasPrefix(importPath, modulePath+"/") {
		return false
	}
	return architecturePatternMatches(pattern, importPath, modulePath)
}

func architectureLayerRuleMatches(pattern, layer string) bool {
	return pattern == "" || pattern == "*" || pattern == layer
}

func architectureBoundaryKind(testOnly bool) string {
	if testOnly {
		return "architecture_test_boundary_violation"
	}
	return "architecture_boundary_violation"
}

func architectureBoundarySeverity(testOnly bool) string {
	if testOnly {
		return "warning"
	}
	return "error"
}

func architectureIsTestImport(imp ImportEdge) bool {
	return strings.HasSuffix(imp.FromPath, "_test.go")
}

func architectureEffectKind(rule ArchitectureEffectRule, fallback string) string {
	if rule.Name == "" {
		return fallback
	}
	if strings.HasPrefix(rule.Name, "architecture_") {
		return rule.Name
	}
	return "architecture_" + rule.Name
}

func architectureEffectSeverity(rule ArchitectureEffectRule) string {
	if rule.Severity != "" {
		return rule.Severity
	}
	return "error"
}

func architectureEffectAllowed(rule ArchitectureEffectRule) bool {
	return rule.Action == ArchitectureRuleAllow
}

func architectureEffectReason(rule ArchitectureEffectRule, fallback string) string {
	if rule.Reason != "" {
		return rule.Reason
	}
	return fallback
}

func architectureExceptionMatches(exceptions []ArchitectureException, kind, pkg, importPath, symbol, fromLayer, toLayer, modulePath string) bool {
	for _, exception := range exceptions {
		if exception.Reason == "" {
			continue
		}
		if exception.Kind != "" && exception.Kind != kind {
			continue
		}
		if exception.Package != "" && !architecturePatternMatches(exception.Package, pkg, modulePath) {
			continue
		}
		if exception.Import != "" && !architecturePatternMatches(exception.Import, importPath, "") {
			continue
		}
		if exception.Symbol != "" && exception.Symbol != symbol {
			continue
		}
		if exception.FromLayer != "" && exception.FromLayer != fromLayer {
			continue
		}
		if exception.ToLayer != "" && exception.ToLayer != toLayer {
			continue
		}
		return true
	}
	return false
}

func architectureReviewedFanOutReason(reviewed []ArchitecturePackageNote, importPath, modulePath string) (string, bool) {
	for _, note := range reviewed {
		if note.Reason != "" && architecturePatternMatches(note.Package, importPath, modulePath) {
			return note.Reason, true
		}
	}
	return "", false
}

func architectureImportAliases(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, spec := range file.Imports {
		importPath := strings.Trim(spec.Path.Value, `"`)
		if spec.Name != nil {
			if spec.Name.Name == "." || spec.Name.Name == "_" {
				continue
			}
			out[spec.Name.Name] = importPath
			continue
		}
		out[path.Base(importPath)] = importPath
	}
	return out
}

func architectureStringIn(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func architectureInModule(importPath, modulePath string) bool {
	return importPath == modulePath || strings.HasPrefix(importPath, modulePath+"/")
}
