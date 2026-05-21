// Package codegate provides an agent-oriented engine for source lookup,
// structural assessment, suggestions, explicit edits, validation, and
// diff-before-commit workflows.
//
// The primary entry point is New, which builds an Engine from explicit roots,
// a Source or fs.FS workspace, and one or more language backends. Use
// language/golang and language/markdown for the built-in backend wrappers.
package codegate
