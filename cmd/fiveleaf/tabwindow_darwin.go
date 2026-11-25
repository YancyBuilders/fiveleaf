package main

import (
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

func focusTabWindowContent(win *mygo.Window) {
	native := objc.ID(win.NativeHandle())
	children := native.Send(objc.RegisterName("contentView")).Send(objc.RegisterName("subviews"))
	for index := range objc.Send[int](children, objc.RegisterName("count")) {
		view := children.Send(objc.RegisterName("objectAtIndex:"), index)
		if objc.Send[bool](view, objc.RegisterName("respondsToSelector:"), objc.RegisterName("insertText:replacementRange:")) {
			native.Send(objc.RegisterName("makeFirstResponder:"), view)
			return
		}
	}
}
