package editor

import "github.com/codewandler/editor/internal/core"

type (
	Operation         = core.Operation
	OperationKind     = core.OperationKind
	TextEdit          = core.TextEdit
	FileEdit          = core.FileEdit
	ReplaceFunction   = core.ReplaceFunction
	ReplaceSymbol     = core.ReplaceSymbol
	RenameSymbol      = core.RenameSymbol
	AppendFunction    = core.AppendFunction
	AppendSymbol      = core.AppendSymbol
	DeleteSymbol      = core.DeleteSymbol
	DeleteFunction    = core.DeleteFunction
	ReplaceMethod     = core.ReplaceMethod
	DeleteMethod      = core.DeleteMethod
	ReplaceComment    = core.ReplaceComment
	EnsureGoStructTag = core.EnsureGoStructTag
	RemoveGoStructTag = core.RemoveGoStructTag
	EnsureGoImport    = core.EnsureGoImport
	RemoveGoImport    = core.RemoveGoImport
	RenameGoImport    = core.RenameGoImport
	MoveSymbol        = core.MoveSymbol
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
	OpEnsureGoImport  = core.OpEnsureGoImport
	OpRemoveGoImport  = core.OpRemoveGoImport
	OpRenameGoImport  = core.OpRenameGoImport
	OpMoveSymbol      = core.OpMoveSymbol
)
