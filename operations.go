package codegate

import "github.com/fluxplane/codegate/internal/core"

// Operation aliases expose supported edit operations from the public package
// while keeping the concrete model in internal/core.
type (
	Operation                 = core.Operation
	OperationKind             = core.OperationKind
	TextEdit                  = core.TextEdit
	FileEdit                  = core.FileEdit
	ReplaceFunction           = core.ReplaceFunction
	ReplaceSymbol             = core.ReplaceSymbol
	RenameSymbol              = core.RenameSymbol
	AppendFunction            = core.AppendFunction
	AppendSymbol              = core.AppendSymbol
	DeleteSymbol              = core.DeleteSymbol
	DeleteFunction            = core.DeleteFunction
	ReplaceMethod             = core.ReplaceMethod
	DeleteMethod              = core.DeleteMethod
	ReplaceComment            = core.ReplaceComment
	EnsureGoStructTag         = core.EnsureGoStructTag
	RemoveGoStructTag         = core.RemoveGoStructTag
	EnsureGoImport            = core.EnsureGoImport
	RemoveGoImport            = core.RemoveGoImport
	RenameGoImport            = core.RenameGoImport
	RenameGoModulePath        = core.RenameGoModulePath
	MoveSymbol                = core.MoveSymbol
	AddGoParameter            = core.AddGoParameter
	RemoveGoParameter         = core.RemoveGoParameter
	RenameGoParameter         = core.RenameGoParameter
	AddGoStructField          = core.AddGoStructField
	RemoveGoStructField       = core.RemoveGoStructField
	RenameGoStructField       = core.RenameGoStructField
	ChangeGoParameterType     = core.ChangeGoParameterType
	ChangeGoResultType        = core.ChangeGoResultType
	RenameGoReceiver          = core.RenameGoReceiver
	AddGoInterfaceMethod      = core.AddGoInterfaceMethod
	RemoveGoInterfaceMethod   = core.RemoveGoInterfaceMethod
	ExtractGoFunction         = core.ExtractGoFunction
	ExtractGoMethod           = core.ExtractGoMethod
	EnsureMarkdownH1          = core.EnsureMarkdownH1
	SetMarkdownHeadingLevel   = core.SetMarkdownHeadingLevel
	InsertMarkdownSectionBody = core.InsertMarkdownSectionBody
	RenameMarkdownHeading     = core.RenameMarkdownHeading
)

// Operation kind aliases identify the edit operation encoded in an Operation.
const (
	OpRenameSymbol              = core.OpRenameSymbol
	OpReplaceSymbol             = core.OpReplaceSymbol
	OpDeleteSymbol              = core.OpDeleteSymbol
	OpReadSymbol                = core.OpReadSymbol
	OpAppendSymbol              = core.OpAppendSymbol
	OpReplaceFunction           = core.OpReplaceFunction
	OpAppendFunction            = core.OpAppendFunction
	OpDeleteFunction            = core.OpDeleteFunction
	OpReplaceMethod             = core.OpReplaceMethod
	OpDeleteMethod              = core.OpDeleteMethod
	OpReplaceComment            = core.OpReplaceComment
	OpEnsureStructTag           = core.OpEnsureStructTag
	OpRemoveStructTag           = core.OpRemoveStructTag
	OpEnsureGoImport            = core.OpEnsureGoImport
	OpRemoveGoImport            = core.OpRemoveGoImport
	OpRenameGoImport            = core.OpRenameGoImport
	OpRenameGoModulePath        = core.OpRenameGoModulePath
	OpMoveSymbol                = core.OpMoveSymbol
	OpAddGoParameter            = core.OpAddGoParameter
	OpRemoveGoParam             = core.OpRemoveGoParam
	OpRenameGoParam             = core.OpRenameGoParam
	OpAddGoField                = core.OpAddGoField
	OpRemoveGoField             = core.OpRemoveGoField
	OpRenameGoField             = core.OpRenameGoField
	OpChangeGoParam             = core.OpChangeGoParam
	OpChangeGoResult            = core.OpChangeGoResult
	OpRenameGoRecv              = core.OpRenameGoRecv
	OpAddGoIfaceMeth            = core.OpAddGoIfaceMeth
	OpRemoveGoIface             = core.OpRemoveGoIface
	OpExtractGoFunc             = core.OpExtractGoFunc
	OpExtractGoMethod           = core.OpExtractGoMethod
	OpMarkdownEnsureH1          = core.OpMarkdownEnsureH1
	OpMarkdownSetHeadingLevel   = core.OpMarkdownSetHeadingLevel
	OpMarkdownInsertSectionBody = core.OpMarkdownInsertSectionBody
	OpMarkdownRenameHeading     = core.OpMarkdownRenameHeading
)

// HasOperations reports whether a refactor proposal carries concrete edits that
// can be passed to ChangeSet.Apply.
func HasOperations(proposal Proposal) bool {
	return len(proposal.Operations) > 0
}

// ExecutableProposals filters advisory-only proposals out of a suggestion list.
func ExecutableProposals(proposals []Proposal) []Proposal {
	out := make([]Proposal, 0, len(proposals))
	for _, proposal := range proposals {
		if HasOperations(proposal) {
			out = append(out, proposal)
		}
	}
	return out
}
