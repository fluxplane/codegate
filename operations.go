package editor

import "github.com/codewandler/editor/internal/core"

type (
	Operation         = core.Operation
	OperationKind     = core.OperationKind
	TextEdit          = core.TextEdit
	FileEdit          = core.FileEdit
	ReplaceFunction   = core.ReplaceFunction
	RenameSymbol      = core.RenameSymbol
	AppendFunction    = core.AppendFunction
	DeleteSymbol      = core.DeleteSymbol
	ReplaceComment    = core.ReplaceComment
	EnsureGoStructTag = core.EnsureGoStructTag
	RemoveGoStructTag = core.RemoveGoStructTag
)

const (
	OpRenameSymbol    = core.OpRenameSymbol
	OpReplaceSymbol   = core.OpReplaceSymbol
	OpDeleteSymbol    = core.OpDeleteSymbol
	OpReadSymbol      = core.OpReadSymbol
	OpAppendSymbol    = core.OpAppendSymbol
	OpReplaceFunction = core.OpReplaceFunction
	OpAppendFunction  = core.OpAppendFunction
	OpDeleteFunction  = core.OpDeleteFunction
	OpReplaceMethod   = core.OpReplaceMethod
	OpDeleteMethod    = core.OpDeleteMethod
	OpReplaceComment  = core.OpReplaceComment
	OpEnsureStructTag = core.OpEnsureStructTag
	OpRemoveStructTag = core.OpRemoveStructTag
)
