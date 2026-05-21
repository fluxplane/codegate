package goast

import (
	"fmt"
	"go/ast"
	"go/token"
	"path"
	"strings"
)

const (
	goComplexityThreshold      = 10
	goNestingThreshold         = 4
	goFunctionLOCThreshold     = 80
	goParameterCountThreshold  = 5
	goReturnCountThreshold     = 5
	goFileLOCThreshold         = 500
	goStructFieldThreshold     = 20
	goInterfaceMethodThreshold = 8
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
	IgnoredErrorCount           int
	UncheckedTypeAssertionCount int
	DeferInLoopCount            int
	ProcessExitCount            int
	StringConcatInLoopCount     int
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
	r.metrics.IgnoredErrorCount += next.metrics.IgnoredErrorCount
	r.metrics.UncheckedTypeAssertionCount += next.metrics.UncheckedTypeAssertionCount
	r.metrics.DeferInLoopCount += next.metrics.DeferInLoopCount
	r.metrics.ProcessExitCount += next.metrics.ProcessExitCount
	r.metrics.StringConcatInLoopCount += next.metrics.StringConcatInLoopCount
}

func (m goQualityMetrics) assessmentMetrics() map[string]interface{} {
	return map[string]interface{}{
		"max_cyclomatic_complexity":      m.MaxCyclomaticComplexity,
		"max_nesting_depth":              m.MaxNestingDepth,
		"max_function_loc":               m.MaxFunctionLOC,
		"large_function_count":           m.LargeFunctionCount,
		"high_complexity_function_count": m.HighComplexityFunctionCount,
		"ignored_error_count":            m.IgnoredErrorCount,
		"unchecked_type_assertion_count": m.UncheckedTypeAssertionCount,
		"defer_in_loop_count":            m.DeferInLoopCount,
		"process_exit_count":             m.ProcessExitCount,
		"string_concat_in_loop_count":    m.StringConcatInLoopCount,
	}
}

func collectGoQuality(pf parsedFile) goQualityReport {
	if isVendorPath(pf.path) || isGeneratedGoSource(pf.src) {
		return goQualityReport{}
	}
	collector := goQualityCollector{
		pf:          pf,
		commentLine: commentLineSet(pf.fset, pf.file),
		imports:     importAliases(pf.file),
	}
	collector.collectFile()
	collector.collectDeclarations()
	collector.collectFunctionSmells()
	return collector.report
}

type goQualityCollector struct {
	pf          parsedFile
	commentLine map[int]bool
	imports     map[string]string
	report      goQualityReport
}

func (c *goQualityCollector) collectFile() {
	loc := nonCommentLOC(c.pf.src, c.commentLine, 0, len(c.pf.src))
	if loc <= goFileLOCThreshold {
		return
	}
	c.addFinding("quality_large_file", "info", Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, c.pf.file.Pos(), c.pf.file.End())}, "",
		fmt.Sprintf("File has %d non-comment lines; consider splitting responsibilities.", loc),
		map[string]float64{"loc": float64(loc), "threshold": goFileLOCThreshold})
}

func (c *goQualityCollector) collectDeclarations() {
	for _, decl := range c.pf.file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			q := c.functionQuality(d)
			c.recordFunctionQuality(q)
		case *ast.GenDecl:
			c.collectTypeQuality(d)
		}
	}
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
			if fields > goStructFieldThreshold {
				c.addFinding("quality_large_struct", "info", Location{URI: c.pf.path, Range: declRange(c.pf.fset, gen, ts)}, ts.Name.Name,
					fmt.Sprintf("Struct %s has %d fields; consider extracting cohesive subtypes.", ts.Name.Name, fields),
					map[string]float64{"fields": float64(fields), "threshold": goStructFieldThreshold})
			}
		case *ast.InterfaceType:
			methods := fieldCount(typ.Methods)
			if methods > goInterfaceMethodThreshold {
				c.addFinding("quality_broad_interface", "info", Location{URI: c.pf.path, Range: declRange(c.pf.fset, gen, ts)}, ts.Name.Name,
					fmt.Sprintf("Interface %s has %d methods; consider smaller consumer-owned interfaces.", ts.Name.Name, methods),
					map[string]float64{"methods": float64(methods), "threshold": goInterfaceMethodThreshold})
			}
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
		case *ast.CaseClause:
			q.complexity++
		case *ast.CommClause:
			q.complexity++
		case *ast.BinaryExpr:
			if x.Op == token.LAND || x.Op == token.LOR {
				q.complexity++
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
		c.collectStmtSmells(fn.Body, 0)
	}
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
		c.collectStmtSmells(x.Body, loopDepth+1)
	case *ast.RangeStmt:
		c.collectExprSmells(x.X, loopDepth)
		c.collectStmtSmells(x.Body, loopDepth+1)
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
	if loopDepth > 0 && assign.Tok == token.ADD_ASSIGN {
		c.report.metrics.StringConcatInLoopCount++
		c.addFinding("performance_string_concat_in_loop", "info", Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, assign.Pos(), assign.End())}, "",
			"Compound append-style assignment inside a loop may repeatedly allocate when used for strings.",
			map[string]float64{"count": 1})
	}
	if hasBlankLHS(assign) && hasCallRHS(assign) {
		c.report.metrics.IgnoredErrorCount++
		c.addFinding("safety_ignored_error", "warning", Location{URI: c.pf.path, Range: rangeOf(c.pf.fset, assign.Pos(), assign.End())}, "",
			"Assignment discards a call result with blank identifier; verify no error is ignored.",
			map[string]float64{"count": 1})
	}
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
