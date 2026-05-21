package goast

import (
	"fmt"
	"go/ast"
	"go/token"
	"path"
	"sort"
	"strings"
)

const (
	goComplexityThreshold        = 10
	goNestingThreshold           = 4
	goFunctionLOCThreshold       = 80
	goParameterCountThreshold    = 5
	goReturnCountThreshold       = 5
	goFileLOCThreshold           = 500
	goStructFieldThreshold       = 20
	goInterfaceMethodThreshold   = 8
	goDocCoverageThreshold       = 80
	goTestToCodeRatioThreshold   = 10
	goBranchDensityThreshold     = 120
	goGeneratedLOCPercentWarning = 25
)

type goQualityReport struct {
	findings []Finding
	metrics  goQualityMetrics
}

type goQualityMetrics struct {
	MaxCyclomaticComplexity     int
	MaxNestingDepth             int
	MaxFunctionLOC              int
	LargeFunctionCount          int
	HighComplexityFunctionCount int
	TotalBranchCount            int
	TotalNonCommentLOC          int
	GeneratedLOC                int
	CodeLOC                     int
	TestLOC                     int
	TestFileCount               int
	TestFunctionCount           int
	TableTestCount              int
	FlakyTestSmellCount         int
	TotalSymbolCount            int
	ExportedSymbolCount         int
	DocumentedExportCount       int
	UndocumentedExportCount     int
	WeakPackageNameCount        int
	WeakIdentifierCount         int
	WeakPackageUnits            map[string]bool
	IgnoredErrorCount           int
	UncheckedTypeAssertionCount int
	DeferInLoopCount            int
	ProcessExitCount            int
	StringConcatInLoopCount     int
	UnsafeUsageCount            int
	WeakCryptoCount             int
	DynamicExecCount            int
	SQLConcatCount              int
	PathRiskCount               int
	ReflectUsageCount           int
	MissingCapacityCount        int
	LargeRangeCopyCount         int
	PackageLOC                  map[string]int
	PackageFileCount            map[string]int
}

type goFunctionQuality struct {
	name       string
	location   Location
	complexity int
	nesting    int
	loc        int
	params     int
	returns    int
}

func (r *goQualityReport) merge(next goQualityReport) {
	r.findings = append(r.findings, next.findings...)
	r.metrics.MaxCyclomaticComplexity = maxInt(r.metrics.MaxCyclomaticComplexity, next.metrics.MaxCyclomaticComplexity)
	r.metrics.MaxNestingDepth = maxInt(r.metrics.MaxNestingDepth, next.metrics.MaxNestingDepth)
	r.metrics.MaxFunctionLOC = maxInt(r.metrics.MaxFunctionLOC, next.metrics.MaxFunctionLOC)
	r.metrics.LargeFunctionCount += next.metrics.LargeFunctionCount
	r.metrics.HighComplexityFunctionCount += next.metrics.HighComplexityFunctionCount
	r.metrics.TotalBranchCount += next.metrics.TotalBranchCount
	r.metrics.TotalNonCommentLOC += next.metrics.TotalNonCommentLOC
	r.metrics.GeneratedLOC += next.metrics.GeneratedLOC
	r.metrics.CodeLOC += next.metrics.CodeLOC
	r.metrics.TestLOC += next.metrics.TestLOC
	r.metrics.TestFileCount += next.metrics.TestFileCount
	r.metrics.TestFunctionCount += next.metrics.TestFunctionCount
	r.metrics.TableTestCount += next.metrics.TableTestCount
	r.metrics.FlakyTestSmellCount += next.metrics.FlakyTestSmellCount
	r.metrics.TotalSymbolCount += next.metrics.TotalSymbolCount
	r.metrics.ExportedSymbolCount += next.metrics.ExportedSymbolCount
	r.metrics.DocumentedExportCount += next.metrics.DocumentedExportCount
	r.metrics.UndocumentedExportCount += next.metrics.UndocumentedExportCount
	r.metrics.WeakIdentifierCount += next.metrics.WeakIdentifierCount
	r.metrics.IgnoredErrorCount += next.metrics.IgnoredErrorCount
	r.metrics.UncheckedTypeAssertionCount += next.metrics.UncheckedTypeAssertionCount
	r.metrics.DeferInLoopCount += next.metrics.DeferInLoopCount
	r.metrics.ProcessExitCount += next.metrics.ProcessExitCount
	r.metrics.StringConcatInLoopCount += next.metrics.StringConcatInLoopCount
	r.metrics.UnsafeUsageCount += next.metrics.UnsafeUsageCount
	r.metrics.WeakCryptoCount += next.metrics.WeakCryptoCount
	r.metrics.DynamicExecCount += next.metrics.DynamicExecCount
	r.metrics.SQLConcatCount += next.metrics.SQLConcatCount
	r.metrics.PathRiskCount += next.metrics.PathRiskCount
	r.metrics.ReflectUsageCount += next.metrics.ReflectUsageCount
	r.metrics.MissingCapacityCount += next.metrics.MissingCapacityCount
	r.metrics.LargeRangeCopyCount += next.metrics.LargeRangeCopyCount
	mergeBoolMaps(&r.metrics.WeakPackageUnits, next.metrics.WeakPackageUnits)
	mergeIntMaps(&r.metrics.PackageLOC, next.metrics.PackageLOC)
	mergeIntMaps(&r.metrics.PackageFileCount, next.metrics.PackageFileCount)
}

func (r *goQualityReport) finalize(includeTests bool) {
	r.metrics.WeakPackageNameCount = len(r.metrics.WeakPackageUnits)
	for _, unit := range sortedBoolMapKeys(r.metrics.WeakPackageUnits) {
		r.findings = append(r.findings, Finding{
			Kind:     "quality_weak_package_name",
			Severity: "info",
			Package:  unit,
			Symbol:   path.Base(packageDir(unit)),
			Reason:   fmt.Sprintf("Package path segment %q is too vague for agents to infer ownership.", path.Base(packageDir(unit))),
			Evidence: []Evidence{{Kind: "quality_weak_package_name", Metrics: map[string]float64{"count": 1}}},
		})
	}
	m := r.metrics
	if m.ExportedSymbolCount > 0 {
		docCoverage := percent(m.DocumentedExportCount, m.ExportedSymbolCount)
		if docCoverage < goDocCoverageThreshold {
			r.findings = append(r.findings, Finding{
				Kind:     "quality_low_doc_coverage",
				Severity: "info",
				Reason:   fmt.Sprintf("Exported API doc coverage is %d%%; agents rely on comments to infer safe changes.", docCoverage),
				Evidence: []Evidence{{Kind: "quality_low_doc_coverage", Metrics: map[string]float64{"coverage_percent": float64(docCoverage), "threshold": goDocCoverageThreshold}}},
			})
		}
	}
	if m.TotalNonCommentLOC > 0 {
		branchDensity := m.TotalBranchCount * 1000 / m.TotalNonCommentLOC
		if branchDensity > goBranchDensityThreshold {
			r.findings = append(r.findings, Finding{
				Kind:     "quality_high_branch_density",
				Severity: "info",
				Reason:   fmt.Sprintf("Branch density is %d per KLOC; review control-flow complexity.", branchDensity),
				Evidence: []Evidence{{Kind: "quality_high_branch_density", Metrics: map[string]float64{"branch_density_per_kloc": float64(branchDensity), "threshold": goBranchDensityThreshold}}},
			})
		}
		generatedPercent := percent(m.GeneratedLOC, m.TotalNonCommentLOC)
		if generatedPercent > goGeneratedLOCPercentWarning {
			r.findings = append(r.findings, Finding{
				Kind:     "quality_high_generated_ratio",
				Severity: "info",
				Reason:   fmt.Sprintf("Generated code is %d%% of indexed non-comment LOC.", generatedPercent),
				Evidence: []Evidence{{Kind: "quality_high_generated_ratio", Metrics: map[string]float64{"generated_loc_percent": float64(generatedPercent), "threshold": goGeneratedLOCPercentWarning}}},
			})
		}
	}
	if includeTests && m.CodeLOC > 0 {
		testRatio := percent(m.TestLOC, m.CodeLOC)
		if m.TestFunctionCount == 0 {
			r.findings = append(r.findings, Finding{
				Kind:     "coverage_no_go_tests",
				Severity: "warning",
				Reason:   "No Go test functions were indexed for the selected scope.",
				Evidence: []Evidence{{Kind: "coverage_no_go_tests", Metrics: map[string]float64{"test_functions": 0}}},
			})
		} else if testRatio < goTestToCodeRatioThreshold {
			r.findings = append(r.findings, Finding{
				Kind:     "coverage_low_test_to_code_ratio",
				Severity: "info",
				Reason:   fmt.Sprintf("Go test/code LOC ratio is %d%%.", testRatio),
				Evidence: []Evidence{{Kind: "coverage_low_test_to_code_ratio", Metrics: map[string]float64{"test_to_code_ratio": float64(testRatio), "threshold": goTestToCodeRatioThreshold}}},
			})
		}
	}
}

func (m goQualityMetrics) assessmentMetrics() map[string]interface{} {
	exportedRatio := 0
	if m.TotalSymbolCount > 0 {
		exportedRatio = percent(m.ExportedSymbolCount, m.TotalSymbolCount)
	}
	docCoverage := 100
	if m.ExportedSymbolCount > 0 {
		docCoverage = percent(m.DocumentedExportCount, m.ExportedSymbolCount)
	}
	testRatio := 0
	if m.CodeLOC > 0 {
		testRatio = percent(m.TestLOC, m.CodeLOC)
	}
	branchDensity := 0
	if m.TotalNonCommentLOC > 0 {
		branchDensity = m.TotalBranchCount * 1000 / m.TotalNonCommentLOC
	}
	generatedPercent := 0
	if m.TotalNonCommentLOC > 0 {
		generatedPercent = percent(m.GeneratedLOC, m.TotalNonCommentLOC)
	}
	return map[string]interface{}{
		"max_cyclomatic_complexity":      m.MaxCyclomaticComplexity,
		"max_nesting_depth":              m.MaxNestingDepth,
		"max_function_loc":               m.MaxFunctionLOC,
		"large_function_count":           m.LargeFunctionCount,
		"high_complexity_function_count": m.HighComplexityFunctionCount,
		"package_loc":                    m.PackageLOC,
		"package_file_count":             m.PackageFileCount,
		"exported_symbol_count":          m.ExportedSymbolCount,
		"exported_ratio":                 exportedRatio,
		"branch_density_per_kloc":        branchDensity,
		"generated_loc_percent":          generatedPercent,
		"doc_coverage_percent":           docCoverage,
		"undocumented_export_count":      m.UndocumentedExportCount,
		"test_file_count":                m.TestFileCount,
		"test_function_count":            m.TestFunctionCount,
		"test_to_code_ratio":             testRatio,
		"table_test_count":               m.TableTestCount,
		"flaky_test_smell_count":         m.FlakyTestSmellCount,
		"weak_package_name_count":        m.WeakPackageNameCount,
		"weak_identifier_count":          m.WeakIdentifierCount,
		"ignored_error_count":            m.IgnoredErrorCount,
		"unchecked_type_assertion_count": m.UncheckedTypeAssertionCount,
		"defer_in_loop_count":            m.DeferInLoopCount,
		"process_exit_count":             m.ProcessExitCount,
		"string_concat_in_loop_count":    m.StringConcatInLoopCount,
		"unsafe_usage_count":             m.UnsafeUsageCount,
		"weak_crypto_count":              m.WeakCryptoCount,
		"dynamic_exec_count":             m.DynamicExecCount,
		"sql_concat_count":               m.SQLConcatCount,
		"path_risk_count":                m.PathRiskCount,
		"reflect_usage_count":            m.ReflectUsageCount,
		"missing_capacity_count":         m.MissingCapacityCount,
		"large_range_copy_count":         m.LargeRangeCopyCount,
	}
}

func collectGoQuality(pf parsedFile) goQualityReport {
	if isVendorPath(pf.path) {
		return goQualityReport{}
	}
	collector := goQualityCollector{
		pf:           pf,
		commentLine:  commentLineSet(pf.fset, pf.file),
		imports:      importAliases(pf.file),
		structFields: structFieldCounts(pf.file),
		generated:    isGeneratedGoSource(pf.src),
		testFile:     isGoTestPath(pf.path),
	}
	collector.collectFile()
	if collector.generated {
		return collector.report
	}
	collector.collectDeclarations()
	collector.collectImportSmells()
	collector.collectFunctionSmells()
	return collector.report
}

type goQualityCollector struct {
	pf                  parsedFile
	commentLine         map[int]bool
	imports             map[string]string
	structFields        map[string]int
	noCapSliceVars      map[string]bool
	missingCapacitySeen map[string]bool
	loopCapacitySources []string
	generated           bool
	testFile            bool
	report              goQualityReport
}

func (c *goQualityCollector) collectFile() {
	loc := nonCommentLOC(c.pf.src, c.commentLine, 0, len(c.pf.src))
	c.report.metrics.TotalNonCommentLOC += loc
	c.addPackageMetric(loc)
	if c.generated {
		c.report.metrics.GeneratedLOC += loc
		return
	}
	if c.testFile {
		c.report.metrics.TestLOC += loc
		c.report.metrics.TestFileCount++
	} else {
		c.report.metrics.CodeLOC += loc
	}
	if weakName(path.Base(packageDir(c.pf.unit))) {
		if c.report.metrics.WeakPackageUnits == nil {
			c.report.metrics.WeakPackageUnits = map[string]bool{}
		}
		c.report.metrics.WeakPackageUnits[c.pf.unit] = true
	}
	if loc <= goFileLOCThreshold {
		return
	}
	c.addFinding("quality_large_file", "info", Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, c.pf.file.Pos(), c.pf.file.End())}, "",
		fmt.Sprintf("File has %d non-comment lines; consider splitting responsibilities.", loc),
		map[string]float64{"loc": float64(loc), "threshold": goFileLOCThreshold})
}

func (c *goQualityCollector) addPackageMetric(loc int) {
	if c.report.metrics.PackageLOC == nil {
		c.report.metrics.PackageLOC = map[string]int{}
	}
	if c.report.metrics.PackageFileCount == nil {
		c.report.metrics.PackageFileCount = map[string]int{}
	}
	c.report.metrics.PackageLOC[c.pf.unit] += loc
	c.report.metrics.PackageFileCount[c.pf.unit]++
}

func (c *goQualityCollector) collectImportSmells() {
	for _, spec := range c.pf.file.Imports {
		importPath := strings.Trim(spec.Path.Value, "\"")
		loc := Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, spec.Pos(), spec.End())}
		switch importPath {
		case "unsafe":
			c.report.metrics.UnsafeUsageCount++
			c.addFinding("security_unsafe_usage", "warning", loc, "",
				"Package imports unsafe; memory safety and pointer invariants need explicit review.",
				map[string]float64{"count": 1})
		case "reflect":
			c.report.metrics.ReflectUsageCount++
			c.addFinding("performance_reflect_usage", "info", loc, "",
				"Package imports reflect; review whether reflection is needed on hot or agent-edited paths.",
				map[string]float64{"count": 1})
		case "crypto/md5", "crypto/sha1", "crypto/des", "crypto/rc4":
			c.report.metrics.WeakCryptoCount++
			c.addFinding("security_weak_crypto", "warning", loc, "",
				fmt.Sprintf("Package imports %s, which is weak or legacy cryptography for security-sensitive use.", importPath),
				map[string]float64{"count": 1})
		}
	}
}

func (c *goQualityCollector) collectDeclarations() {
	for _, decl := range c.pf.file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			c.collectFunctionDeclarationQuality(d)
			q := c.functionQuality(d)
			c.recordFunctionQuality(q)
		case *ast.GenDecl:
			c.collectGenDeclarationQuality(d)
			c.collectTypeQuality(d)
		}
	}
}

func (c *goQualityCollector) collectFunctionDeclarationQuality(fn *ast.FuncDecl) {
	if !c.testFile {
		c.report.metrics.TotalSymbolCount++
	}
	c.collectExportDoc(fn.Name, fn.Doc, Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, fn.Name.Pos(), fn.Name.End())}, fn.Name.Name)
	if c.testFile && c.isRunnableGoTestFunction(fn) {
		c.report.metrics.TestFunctionCount++
		if functionUsesTablePattern(fn) {
			c.report.metrics.TableTestCount++
		}
	}
	if !c.testFile && isExported(fn.Name.Name) && weakName(fn.Name.Name) {
		c.report.metrics.WeakIdentifierCount++
		c.addFinding("quality_weak_identifier_name", "info", Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, fn.Name.Pos(), fn.Name.End())}, fn.Name.Name,
			fmt.Sprintf("Exported identifier %q is too vague for agents to infer intent.", fn.Name.Name),
			map[string]float64{"count": 1})
	}
}

func (c *goQualityCollector) collectGenDeclarationQuality(gen *ast.GenDecl) {
	for _, spec := range gen.Specs {
		switch x := spec.(type) {
		case *ast.TypeSpec:
			if !c.testFile {
				c.report.metrics.TotalSymbolCount++
			}
			c.collectExportDoc(x.Name, firstComment(x.Doc, x.Comment, gen.Doc), Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, x.Name.Pos(), x.Name.End())}, x.Name.Name)
			if !c.testFile && isExported(x.Name.Name) && weakName(x.Name.Name) {
				c.report.metrics.WeakIdentifierCount++
				c.addFinding("quality_weak_identifier_name", "info", Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, x.Name.Pos(), x.Name.End())}, x.Name.Name,
					fmt.Sprintf("Exported identifier %q is too vague for agents to infer intent.", x.Name.Name),
					map[string]float64{"count": 1})
			}
		case *ast.ValueSpec:
			for _, name := range x.Names {
				if !c.testFile {
					c.report.metrics.TotalSymbolCount++
				}
				c.collectExportDoc(name, firstComment(x.Doc, x.Comment, gen.Doc), Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, name.Pos(), name.End())}, name.Name)
				if !c.testFile && isExported(name.Name) && weakName(name.Name) {
					c.report.metrics.WeakIdentifierCount++
					c.addFinding("quality_weak_identifier_name", "info", Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, name.Pos(), name.End())}, name.Name,
						fmt.Sprintf("Exported identifier %q is too vague for agents to infer intent.", name.Name),
						map[string]float64{"count": 1})
				}
			}
		}
	}
}

func (c *goQualityCollector) collectExportDoc(name *ast.Ident, doc *ast.CommentGroup, loc Location, symbol string) {
	if name == nil || !isExported(name.Name) || c.testFile || !isPublicAPIUnit(c.pf.unit) {
		return
	}
	c.report.metrics.ExportedSymbolCount++
	if strings.TrimSpace(commentText(doc)) != "" {
		c.report.metrics.DocumentedExportCount++
		return
	}
	c.report.metrics.UndocumentedExportCount++
	c.addFinding("quality_undocumented_export", "info", loc, symbol,
		fmt.Sprintf("Exported identifier %s has no doc comment.", symbol),
		map[string]float64{"count": 1})
}

func isPublicAPIUnit(unit string) bool {
	dir := packageDir(unit)
	return dir != "cmd" && !strings.HasPrefix(dir, "cmd/") && dir != "internal" && !strings.HasPrefix(dir, "internal/")
}

func (c *goQualityCollector) collectTypeQuality(gen *ast.GenDecl) {
	for _, spec := range gen.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		switch typ := ts.Type.(type) {
		case *ast.StructType:
			fields := fieldCount(typ.Fields)
			c.collectExportedFieldDocs(ts.Name.Name, typ.Fields)
			if fields > goStructFieldThreshold {
				c.addFinding("quality_large_struct", "info", Location{URI: c.pf.path, Range: declRange(c.pf.fset, gen, ts)}, ts.Name.Name,
					fmt.Sprintf("Struct %s has %d fields; consider extracting cohesive subtypes.", ts.Name.Name, fields),
					map[string]float64{"fields": float64(fields), "threshold": goStructFieldThreshold})
			}
		case *ast.InterfaceType:
			methods := fieldCount(typ.Methods)
			c.collectExportedFieldDocs(ts.Name.Name, typ.Methods)
			if methods > goInterfaceMethodThreshold {
				c.addFinding("quality_broad_interface", "info", Location{URI: c.pf.path, Range: declRange(c.pf.fset, gen, ts)}, ts.Name.Name,
					fmt.Sprintf("Interface %s has %d methods; consider smaller consumer-owned interfaces.", ts.Name.Name, methods),
					map[string]float64{"methods": float64(methods), "threshold": goInterfaceMethodThreshold})
			}
		}
	}
}

func (c *goQualityCollector) collectExportedFieldDocs(container string, fields *ast.FieldList) {
	if fields == nil || c.testFile {
		return
	}
	for _, field := range fields.List {
		for _, name := range field.Names {
			c.report.metrics.TotalSymbolCount++
			if !isExported(name.Name) {
				continue
			}
			qname := container + "." + name.Name
			c.collectExportDoc(name, firstComment(field.Doc, field.Comment), Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, name.Pos(), name.End())}, qname)
		}
	}
}

func (c *goQualityCollector) functionQuality(fn *ast.FuncDecl) goFunctionQuality {
	q := goFunctionQuality{
		name:       fn.Name.Name,
		location:   Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, fn.Pos(), fn.End())},
		complexity: 1,
		params:     fieldCount(fn.Type.Params),
		loc:        nonCommentLOC(c.pf.src, c.commentLine, c.pf.fset.Position(fn.Pos()).Offset, c.pf.fset.Position(fn.End()).Offset),
	}
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		q.name = receiverName(fn.Recv.List[0].Type) + "." + fn.Name.Name
	}
	if fn.Body == nil {
		return q
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			q.complexity++
			c.report.metrics.TotalBranchCount++
		case *ast.CaseClause:
			q.complexity++
			c.report.metrics.TotalBranchCount++
		case *ast.CommClause:
			q.complexity++
			c.report.metrics.TotalBranchCount++
		case *ast.BinaryExpr:
			if x.Op == token.LAND || x.Op == token.LOR {
				q.complexity++
				c.report.metrics.TotalBranchCount++
			}
		case *ast.ReturnStmt:
			q.returns++
		}
		return true
	})
	q.nesting = maxControlNesting(fn.Body, 0)
	return q
}

func (c *goQualityCollector) recordFunctionQuality(q goFunctionQuality) {
	c.report.metrics.MaxCyclomaticComplexity = maxInt(c.report.metrics.MaxCyclomaticComplexity, q.complexity)
	c.report.metrics.MaxNestingDepth = maxInt(c.report.metrics.MaxNestingDepth, q.nesting)
	c.report.metrics.MaxFunctionLOC = maxInt(c.report.metrics.MaxFunctionLOC, q.loc)
	if q.complexity > goComplexityThreshold {
		c.report.metrics.HighComplexityFunctionCount++
		c.addFinding("quality_high_complexity_function", "warning", q.location, q.name,
			fmt.Sprintf("Function %s has cyclomatic complexity %d.", q.name, q.complexity),
			map[string]float64{"cyclomatic_complexity": float64(q.complexity), "threshold": goComplexityThreshold})
	}
	if q.nesting > goNestingThreshold {
		c.addFinding("quality_deeply_nested_function", "warning", q.location, q.name,
			fmt.Sprintf("Function %s reaches nesting depth %d.", q.name, q.nesting),
			map[string]float64{"nesting_depth": float64(q.nesting), "threshold": goNestingThreshold})
	}
	if q.loc > goFunctionLOCThreshold {
		c.report.metrics.LargeFunctionCount++
		c.addFinding("quality_large_function", "info", q.location, q.name,
			fmt.Sprintf("Function %s has %d non-comment lines.", q.name, q.loc),
			map[string]float64{"loc": float64(q.loc), "threshold": goFunctionLOCThreshold})
	}
	if q.params > goParameterCountThreshold {
		c.addFinding("quality_many_parameters", "info", q.location, q.name,
			fmt.Sprintf("Function %s has %d parameters.", q.name, q.params),
			map[string]float64{"parameters": float64(q.params), "threshold": goParameterCountThreshold})
	}
	if q.returns > goReturnCountThreshold {
		c.addFinding("quality_many_returns", "info", q.location, q.name,
			fmt.Sprintf("Function %s has %d explicit returns.", q.name, q.returns),
			map[string]float64{"returns": float64(q.returns), "threshold": goReturnCountThreshold})
	}
}

func (c *goQualityCollector) collectFunctionSmells() {
	for _, decl := range c.pf.file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		c.noCapSliceVars = collectNoCapSliceVars(fn.Body)
		c.missingCapacitySeen = map[string]bool{}
		c.loopCapacitySources = nil
		c.collectStmtSmells(fn.Body, 0)
	}
	c.noCapSliceVars = nil
	c.missingCapacitySeen = nil
	c.loopCapacitySources = nil
}

func (c *goQualityCollector) collectStmtSmells(stmt ast.Stmt, loopDepth int) {
	switch x := stmt.(type) {
	case *ast.BlockStmt:
		for _, child := range x.List {
			c.collectStmtSmells(child, loopDepth)
		}
	case *ast.IfStmt:
		if x.Init != nil {
			c.collectStmtSmells(x.Init, loopDepth)
		}
		c.collectExprSmells(x.Cond, loopDepth)
		c.collectStmtSmells(x.Body, loopDepth)
		if x.Else != nil {
			c.collectStmtSmells(x.Else, loopDepth)
		}
	case *ast.ForStmt:
		if x.Init != nil {
			c.collectStmtSmells(x.Init, loopDepth)
		}
		if x.Cond != nil {
			c.collectExprSmells(x.Cond, loopDepth)
		}
		if x.Post != nil {
			c.collectStmtSmells(x.Post, loopDepth)
		}
		c.pushLoopCapacitySource(forLoopCapacitySource(x))
		c.collectStmtSmells(x.Body, loopDepth+1)
		c.popLoopCapacitySource()
	case *ast.RangeStmt:
		c.collectExprSmells(x.X, loopDepth)
		c.collectLargeRangeCopy(x)
		c.pushLoopCapacitySource(rangeLoopCapacitySource(x))
		c.collectStmtSmells(x.Body, loopDepth+1)
		c.popLoopCapacitySource()
	case *ast.SwitchStmt:
		if x.Init != nil {
			c.collectStmtSmells(x.Init, loopDepth)
		}
		if x.Tag != nil {
			c.collectExprSmells(x.Tag, loopDepth)
		}
		c.collectStmtSmells(x.Body, loopDepth)
	case *ast.TypeSwitchStmt:
		if x.Init != nil {
			c.collectStmtSmells(x.Init, loopDepth)
		}
		if x.Assign != nil {
			c.collectStmtSmells(x.Assign, loopDepth)
		}
		c.collectStmtSmells(x.Body, loopDepth)
	case *ast.SelectStmt:
		c.collectStmtSmells(x.Body, loopDepth)
	case *ast.CaseClause:
		for _, expr := range x.List {
			c.collectExprSmells(expr, loopDepth)
		}
		for _, child := range x.Body {
			c.collectStmtSmells(child, loopDepth)
		}
	case *ast.CommClause:
		if x.Comm != nil {
			c.collectStmtSmells(x.Comm, loopDepth)
		}
		for _, child := range x.Body {
			c.collectStmtSmells(child, loopDepth)
		}
	case *ast.DeferStmt:
		if loopDepth > 0 {
			c.report.metrics.DeferInLoopCount++
			c.addFinding("safety_defer_in_loop", "warning", Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, x.Pos(), x.End())}, "",
				"Defer inside a loop delays cleanup until the surrounding function returns.",
				map[string]float64{"count": 1})
		}
		c.collectExprSmells(x.Call, loopDepth)
	case *ast.ExprStmt:
		c.collectExprSmells(x.X, loopDepth)
	case *ast.AssignStmt:
		c.collectAssignSmells(x, loopDepth)
	case *ast.DeclStmt:
		if gen, ok := x.Decl.(*ast.GenDecl); ok {
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, expr := range value.Values {
					c.collectExprSmells(expr, loopDepth)
				}
			}
		}
	case *ast.ReturnStmt:
		for _, expr := range x.Results {
			c.collectExprSmells(expr, loopDepth)
		}
	case *ast.GoStmt:
		c.collectExprSmells(x.Call, loopDepth)
	case *ast.SendStmt:
		c.collectExprSmells(x.Chan, loopDepth)
		c.collectExprSmells(x.Value, loopDepth)
	case *ast.IncDecStmt:
		c.collectExprSmells(x.X, loopDepth)
	}
}

func (c *goQualityCollector) collectAssignSmells(assign *ast.AssignStmt, loopDepth int) {
	for _, rhs := range assign.Rhs {
		c.collectExprSmells(rhs, loopDepth)
	}
	for _, lhs := range assign.Lhs {
		c.collectExprSmells(lhs, loopDepth)
	}
	if loopDepth > 0 && assign.Tok == token.ADD_ASSIGN && addAssignHasStringOperand(assign) {
		c.report.metrics.StringConcatInLoopCount++
		c.addFinding("performance_string_concat_in_loop", "info", Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, assign.Pos(), assign.End())}, "",
			"Compound append-style assignment inside a loop may repeatedly allocate when used for strings.",
			map[string]float64{"count": 1})
	}
	if loopDepth > 0 {
		if target, source := appendTarget(assign), c.currentLoopCapacitySource(); target != "" && source != "" && c.noCapSliceVars[target] && !c.missingCapacitySeen[target] {
			c.missingCapacitySeen[target] = true
			c.report.metrics.MissingCapacityCount++
			c.addFinding("performance_missing_capacity", "info", Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, assign.Pos(), assign.End())}, target,
				fmt.Sprintf("Slice %s is appended to inside a loop with capacity source %s but no obvious capacity hint.", target, source),
				map[string]float64{"count": 1, "capacity_source_detected": 1})
		}
	}
	if hasBlankLHS(assign) && hasCallRHS(assign) {
		c.report.metrics.IgnoredErrorCount++
		c.addFinding("safety_ignored_error", "warning", Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, assign.Pos(), assign.End())}, "",
			"Assignment discards a call result with blank identifier; verify no error is ignored.",
			map[string]float64{"count": 1})
	}
}

func addAssignHasStringOperand(assign *ast.AssignStmt) bool {
	for _, expr := range assign.Rhs {
		if exprContainsStringLiteral(expr) {
			return true
		}
	}
	return false
}

func exprContainsStringLiteral(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if ok && lit.Kind == token.STRING {
			found = true
			return false
		}
		return !found
	})
	return found
}

func (c *goQualityCollector) collectExprSmells(expr ast.Expr, loopDepth int) {
	ast.Inspect(expr, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.TypeAssertExpr:
			if x.Type != nil && !isCommaOKTypeAssertion(c.pf.file, x) {
				c.report.metrics.UncheckedTypeAssertionCount++
				c.addFinding("safety_unchecked_type_assertion", "warning", Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, x.Pos(), x.End())}, "",
					"Type assertion does not use the comma-ok form.",
					map[string]float64{"count": 1})
			}
		case *ast.CallExpr:
			if c.isProcessExitCall(x) {
				c.report.metrics.ProcessExitCount++
				c.addFinding("safety_process_exit", "warning", Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, x.Pos(), x.End())}, "",
					"Process-level exit or panic bypasses normal error handling and cleanup.",
					map[string]float64{"count": 1})
			}
			if c.isDynamicExecCall(x) {
				c.report.metrics.DynamicExecCount++
				c.addFinding("security_dynamic_exec", "warning", Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, x.Pos(), x.End())}, "",
					"Process execution uses a dynamic command or argument; validate the source and quoting boundary.",
					map[string]float64{"count": 1})
			}
			if c.isSQLConcatCall(x) {
				c.report.metrics.SQLConcatCount++
				c.addFinding("security_sql_concat", "warning", Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, x.Pos(), x.End())}, "",
					"SQL execution appears to build the query with string composition instead of parameter binding.",
					map[string]float64{"count": 1})
			}
			if c.isDynamicPathCall(x) {
				c.report.metrics.PathRiskCount++
				c.addFinding("security_dynamic_file_path", "warning", Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, x.Pos(), x.End())}, "",
					"Filesystem access uses a dynamic path; validate traversal and trust boundaries.",
					map[string]float64{"count": 1})
			}
			if c.testFile && c.isFlakyTestCall(x) {
				c.report.metrics.FlakyTestSmellCount++
				c.addFinding("coverage_flaky_test_smell", "info", Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, x.Pos(), x.End())}, "",
					"Test uses sleep, wall-clock, randomness, or network-like calls that can make automation nondeterministic.",
					map[string]float64{"count": 1})
			}
		}
		return true
	})
}

func (c *goQualityCollector) isProcessExitCall(call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name == "panic"
	case *ast.SelectorExpr:
		importPath, ok := c.imports[exprName(fun.X)]
		if !ok {
			return false
		}
		return importPath == "os" && fun.Sel.Name == "Exit" || importPath == "log" && strings.HasPrefix(fun.Sel.Name, "Fatal")
	default:
		return false
	}
}

func (c *goQualityCollector) isFlakyTestCall(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	importPath := c.imports[exprName(selector.X)]
	switch importPath {
	case "time":
		return selector.Sel.Name == "Sleep" || selector.Sel.Name == "Now" || selector.Sel.Name == "After" || selector.Sel.Name == "Tick"
	case "math/rand", "crypto/rand":
		return true
	case "net/http":
		return selector.Sel.Name == "Get" || selector.Sel.Name == "Post" || selector.Sel.Name == "Head" || selector.Sel.Name == "Do"
	default:
		return false
	}
}

func (c *goQualityCollector) isDynamicExecCall(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || c.imports[exprName(selector.X)] != "os/exec" {
		return false
	}
	argStart := 0
	switch selector.Sel.Name {
	case "Command":
		argStart = 0
	case "CommandContext":
		argStart = 1
	default:
		return false
	}
	if len(call.Args) <= argStart {
		return false
	}
	for _, arg := range call.Args[argStart:] {
		if !isStringLiteral(arg) {
			return true
		}
	}
	return false
}

func (c *goQualityCollector) isSQLConcatCall(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || len(call.Args) == 0 {
		return false
	}
	switch selector.Sel.Name {
	case "Exec", "ExecContext", "Query", "QueryContext", "QueryRow", "QueryRowContext":
	default:
		return false
	}
	queryArg := call.Args[0]
	if strings.HasSuffix(selector.Sel.Name, "Context") {
		if len(call.Args) < 2 {
			return false
		}
		queryArg = call.Args[1]
	}
	return containsStringConcat(queryArg) || c.isFmtSprintf(queryArg)
}

func (c *goQualityCollector) isDynamicPathCall(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || len(call.Args) == 0 {
		return false
	}
	importPath := c.imports[exprName(selector.X)]
	argIndex := 0
	switch importPath {
	case "os":
		switch selector.Sel.Name {
		case "Open", "OpenFile", "ReadFile", "WriteFile", "Create", "Remove", "RemoveAll", "Mkdir", "MkdirAll":
		default:
			return false
		}
	case "path/filepath":
		if selector.Sel.Name != "Join" {
			return false
		}
		for _, arg := range call.Args {
			if !isStringLiteral(arg) {
				return true
			}
		}
		return false
	default:
		return false
	}
	return !isStringLiteral(call.Args[argIndex])
}

func (c *goQualityCollector) isFmtSprintf(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == "Sprintf" && c.imports[exprName(selector.X)] == "fmt"
}

func (c *goQualityCollector) collectLargeRangeCopy(stmt *ast.RangeStmt) {
	if ident, ok := stmt.Value.(*ast.Ident); ok && ident.Name == "_" {
		return
	}
	structName := rangeCompositeStructName(stmt.X)
	if structName == "" || c.structFields[structName] <= goStructFieldThreshold {
		return
	}
	c.report.metrics.LargeRangeCopyCount++
	c.addFinding("performance_large_range_copy", "info", Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, stmt.Pos(), stmt.End())}, structName,
		fmt.Sprintf("Range copies values of large struct %s; consider ranging over pointers or indexes on hot paths.", structName),
		map[string]float64{"fields": float64(c.structFields[structName]), "threshold": goStructFieldThreshold})
}

func (c *goQualityCollector) pushLoopCapacitySource(source string) {
	c.loopCapacitySources = append(c.loopCapacitySources, source)
}

func (c *goQualityCollector) popLoopCapacitySource() {
	if len(c.loopCapacitySources) == 0 {
		return
	}
	c.loopCapacitySources = c.loopCapacitySources[:len(c.loopCapacitySources)-1]
}

func (c *goQualityCollector) currentLoopCapacitySource() string {
	for i := len(c.loopCapacitySources) - 1; i >= 0; i-- {
		if c.loopCapacitySources[i] != "" {
			return c.loopCapacitySources[i]
		}
	}
	return ""
}

func (c *goQualityCollector) addFinding(kind, severity string, loc Location, symbol, reason string, metrics map[string]float64) {
	c.report.findings = append(c.report.findings, Finding{
		Kind:     kind,
		Severity: severity,
		Package:  c.pf.unit,
		Location: loc,
		Symbol:   symbol,
		Evidence: []Evidence{{
			Kind:     kind,
			Message:  sourceLine(c.pf.src, loc.Range.Start.Offset),
			Location: loc,
			Metrics:  metrics,
		}},
		Reason: reason,
	})
}

func maxControlNesting(stmt ast.Stmt, depth int) int {
	if stmt == nil {
		return depth
	}
	maxDepth := depth
	var walkStmt func(ast.Stmt, int)
	walkStmt = func(s ast.Stmt, current int) {
		if s == nil {
			return
		}
		next := current
		switch s.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			next++
			maxDepth = maxInt(maxDepth, next)
		}
		ast.Inspect(s, func(n ast.Node) bool {
			if n == nil || n == s {
				return true
			}
			if child, ok := n.(ast.Stmt); ok {
				switch child.(type) {
				case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
					walkStmt(child, next)
					return false
				}
			}
			return true
		})
	}
	walkStmt(stmt, depth)
	return maxDepth
}

func fieldCount(fields *ast.FieldList) int {
	if fields == nil {
		return 0
	}
	n := 0
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			n++
			continue
		}
		n += len(field.Names)
	}
	return n
}

func commentLineSet(fset *token.FileSet, file *ast.File) map[int]bool {
	lines := map[int]bool{}
	for _, group := range file.Comments {
		for _, comment := range group.List {
			start := fset.Position(comment.Pos()).Line
			end := fset.Position(comment.End()).Line
			for line := start; line <= end; line++ {
				lines[line] = true
			}
		}
	}
	return lines
}

func nonCommentLOC(src []byte, commentLines map[int]bool, start, end int) int {
	if start < 0 {
		start = 0
	}
	if end > len(src) {
		end = len(src)
	}
	if start >= end {
		return 0
	}
	loc := 0
	lineStart := 0
	line := 1
	for i := 0; i <= len(src); i++ {
		if i < len(src) && src[i] != '\n' {
			continue
		}
		lineEnd := i
		if lineEnd > start && lineStart < end && !commentLines[line] {
			from := maxInt(lineStart, start)
			to := minInt(lineEnd, end)
			if from < to && strings.TrimSpace(string(src[from:to])) != "" {
				loc++
			}
		}
		lineStart = i + 1
		line++
	}
	return loc
}

func importAliases(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, spec := range file.Imports {
		importPath := strings.Trim(spec.Path.Value, "\"")
		name := path.Base(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name == "." || name == "_" {
			continue
		}
		out[name] = importPath
	}
	return out
}

func isVendorPath(p string) bool {
	return p == "vendor" || strings.HasPrefix(p, "vendor/") || strings.Contains(p, "/vendor/")
}

func isGoTestPath(p string) bool {
	return strings.HasSuffix(p, "_test.go")
}

func (c *goQualityCollector) isRunnableGoTestFunction(fn *ast.FuncDecl) bool {
	prefix, param := runnableTestPrefix(fn.Name.Name)
	if prefix == "" || fn.Recv != nil || fn.Type == nil || fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
		return false
	}
	field := fn.Type.Params.List[0]
	if len(field.Names) > 1 {
		return false
	}
	star, ok := field.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := star.X.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != param {
		return false
	}
	importPath := c.imports[exprName(selector.X)]
	return importPath == "testing"
}

func runnableTestPrefix(name string) (string, string) {
	for _, candidate := range []struct {
		prefix string
		param  string
	}{
		{prefix: "Test", param: "T"},
		{prefix: "Benchmark", param: "B"},
		{prefix: "Fuzz", param: "F"},
	} {
		if isRunnableTestName(name, candidate.prefix) {
			return candidate.prefix, candidate.param
		}
	}
	return "", ""
}

func isRunnableTestName(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	if len(name) == len(prefix) {
		return true
	}
	next := rune(name[len(prefix)])
	return next < 'a' || next > 'z'
}

func functionUsesTablePattern(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found || n == nil {
			return false
		}
		switch x := n.(type) {
		case *ast.RangeStmt:
			if id, ok := x.X.(*ast.Ident); ok && strings.Contains(strings.ToLower(id.Name), "tests") {
				found = true
				return false
			}
		case *ast.CompositeLit:
			if array, ok := x.Type.(*ast.ArrayType); ok {
				if _, ok := array.Elt.(*ast.StructType); ok {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

func weakName(name string) bool {
	switch strings.ToLower(name) {
	case "util", "utils", "common", "misc", "data", "manager", "helper", "helpers":
		return true
	default:
		return false
	}
}

func mergeIntMaps(dst *map[string]int, src map[string]int) {
	if len(src) == 0 {
		return
	}
	if *dst == nil {
		*dst = map[string]int{}
	}
	for key, value := range src {
		(*dst)[key] += value
	}
}

func mergeBoolMaps(dst *map[string]bool, src map[string]bool) {
	if len(src) == 0 {
		return
	}
	if *dst == nil {
		*dst = map[string]bool{}
	}
	for key, value := range src {
		(*dst)[key] = (*dst)[key] || value
	}
}

func sortedBoolMapKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for key, value := range values {
		if value {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func percent(part, total int) int {
	if total <= 0 {
		return 0
	}
	return part * 100 / total
}

func hasBlankLHS(assign *ast.AssignStmt) bool {
	for _, lhs := range assign.Lhs {
		if id, ok := lhs.(*ast.Ident); ok && id.Name == "_" {
			return true
		}
	}
	return false
}

func hasCallRHS(assign *ast.AssignStmt) bool {
	for _, rhs := range assign.Rhs {
		if _, ok := rhs.(*ast.CallExpr); ok {
			return true
		}
	}
	return false
}

func appendTarget(assign *ast.AssignStmt) string {
	if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return ""
	}
	lhs, ok := assign.Lhs[0].(*ast.Ident)
	if !ok || lhs.Name == "_" {
		return ""
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok {
		return ""
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok || fun.Name != "append" || len(call.Args) == 0 {
		return ""
	}
	arg, ok := call.Args[0].(*ast.Ident)
	if !ok || arg.Name != lhs.Name {
		return ""
	}
	return lhs.Name
}

func rangeLoopCapacitySource(stmt *ast.RangeStmt) string {
	source := capacitySourceName(stmt.X)
	if source == "" {
		return ""
	}
	return "len(" + source + ")"
}

func forLoopCapacitySource(stmt *ast.ForStmt) string {
	if stmt == nil || stmt.Cond == nil {
		return ""
	}
	cond, ok := stmt.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.LSS && cond.Op != token.LEQ {
		return ""
	}
	index, ok := cond.X.(*ast.Ident)
	if !ok || index.Name == "_" || !isZeroLoopInit(stmt.Init, index.Name) || !isLoopPostIncrement(stmt.Post, index.Name) {
		return ""
	}
	source := lenCallSourceName(cond.Y)
	if source == "" {
		return ""
	}
	return "len(" + source + ")"
}

func isZeroLoopInit(stmt ast.Stmt, index string) bool {
	assign, ok := stmt.(*ast.AssignStmt)
	if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return false
	}
	lhs, ok := assign.Lhs[0].(*ast.Ident)
	if !ok || lhs.Name != index {
		return false
	}
	lit, ok := assign.Rhs[0].(*ast.BasicLit)
	return ok && lit.Kind == token.INT && lit.Value == "0"
}

func isLoopPostIncrement(stmt ast.Stmt, index string) bool {
	inc, ok := stmt.(*ast.IncDecStmt)
	if !ok || inc.Tok != token.INC {
		return false
	}
	id, ok := inc.X.(*ast.Ident)
	return ok && id.Name == index
}

func lenCallSourceName(expr ast.Expr) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return ""
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok || fun.Name != "len" {
		return ""
	}
	return capacitySourceName(call.Args[0])
}

func capacitySourceName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		prefix := capacitySourceName(x.X)
		if prefix == "" {
			return ""
		}
		return prefix + "." + x.Sel.Name
	default:
		return ""
	}
}

func collectNoCapSliceVars(body *ast.BlockStmt) map[string]bool {
	noCap := map[string]bool{}
	capped := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ValueSpec:
			for i, name := range x.Names {
				if name == nil || name.Name == "_" {
					continue
				}
				if len(x.Values) == 0 {
					if isSliceType(x.Type) {
						noCap[name.Name] = true
					}
					continue
				}
				value := x.Values[minInt(i, len(x.Values)-1)]
				recordSliceCapacityHint(name.Name, value, noCap, capped)
			}
		case *ast.AssignStmt:
			for i, lhs := range x.Lhs {
				name, ok := lhs.(*ast.Ident)
				if !ok || name.Name == "_" || i >= len(x.Rhs) {
					continue
				}
				recordSliceCapacityHint(name.Name, x.Rhs[i], noCap, capped)
			}
		}
		return true
	})
	for name := range capped {
		delete(noCap, name)
	}
	return noCap
}

func recordSliceCapacityHint(name string, expr ast.Expr, noCap, capped map[string]bool) {
	switch {
	case isCapSliceInit(expr):
		capped[name] = true
	case isNoCapSliceInit(expr):
		if !capped[name] {
			noCap[name] = true
		}
	}
}

func isNoCapSliceInit(expr ast.Expr) bool {
	switch x := expr.(type) {
	case *ast.CompositeLit:
		return isSliceType(x.Type)
	case *ast.CallExpr:
		fun, ok := x.Fun.(*ast.Ident)
		return ok && fun.Name == "make" && len(x.Args) >= 2 && len(x.Args) < 3 && isSliceType(x.Args[0])
	default:
		return false
	}
}

func isCapSliceInit(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	fun, ok := call.Fun.(*ast.Ident)
	return ok && fun.Name == "make" && len(call.Args) >= 3 && isSliceType(call.Args[0])
}

func isSliceType(expr ast.Expr) bool {
	array, ok := expr.(*ast.ArrayType)
	return ok && array.Len == nil
}

func containsStringConcat(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found || n == nil {
			return false
		}
		if binary, ok := n.(*ast.BinaryExpr); ok && binary.Op == token.ADD {
			found = true
			return false
		}
		return true
	})
	return found
}

func isStringLiteral(expr ast.Expr) bool {
	lit, ok := expr.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING
}

func structFieldCounts(file *ast.File) map[string]int {
	out := map[string]int{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			out[ts.Name.Name] = fieldCount(st.Fields)
		}
	}
	return out
}

func rangeCompositeStructName(expr ast.Expr) string {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return ""
	}
	array, ok := lit.Type.(*ast.ArrayType)
	if !ok {
		return ""
	}
	switch elt := array.Elt.(type) {
	case *ast.Ident:
		return elt.Name
	case *ast.StarExpr:
		return ""
	default:
		return ""
	}
}

func isCommaOKTypeAssertion(file *ast.File, target *ast.TypeAssertExpr) bool {
	ok := false
	ast.Inspect(file, func(n ast.Node) bool {
		if ok || n == nil {
			return false
		}
		assign, isAssign := n.(*ast.AssignStmt)
		if !isAssign || len(assign.Lhs) < 2 || len(assign.Rhs) != 1 {
			return true
		}
		if assign.Rhs[0] == target {
			ok = true
			return false
		}
		return true
	})
	return ok
}

func exprName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	default:
		return ""
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
