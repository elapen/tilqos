//go:build windows && (amd64 || arm64)

package winapp

// Where the text caret is, so the suggestion appears next to what the user is typing
// (the counterpart of CaretLocator.swift).
//
//  1. The system caret (GetGUIThreadInfo): Win32 edit controls, rich edits, most classic
//     applications, and Chromium, which keeps a system caret for IMEs and magnifiers.
//  2. MSAA (OBJID_CARET → IAccessible::accLocation) for applications that draw their own
//     caret but implement accessibility. That call crosses into the target process and
//     can block if it hangs, so it runs on a dedicated COM thread with a deadline.
//  3. Otherwise none: the popup goes next to the mouse pointer.

import (
	"runtime"
	"time"

	"tilqos/internal/win32"
)

type caretReply struct {
	rect win32.RECT
	ok   bool
}

type caretRequest struct {
	window uintptr
	reply  chan caretReply
}

type caretLocator struct {
	requests chan caretRequest
}

func newCaretLocator() *caretLocator {
	c := &caretLocator{requests: make(chan caretRequest)}
	go c.serve()
	return c
}

func (c *caretLocator) serve() {
	runtime.LockOSThread()
	if !win32.CoInitializeMultithreaded() {
		// Without COM there is no MSAA; requests simply time out to the fallback.
		for request := range c.requests {
			request.reply <- caretReply{}
		}
		return
	}
	for request := range c.requests {
		rect, ok := win32.CaretFromAccessibility(request.window)
		request.reply <- caretReply{rect, ok}
	}
}

// Locate returns the caret rectangle in screen coordinates.
func (c *caretLocator) Locate(window uintptr) (win32.RECT, bool) {
	thread, _ := win32.GetWindowThreadProcessId(window)
	if thread == 0 {
		// The window is gone (GetGUIThreadInfo(0) would describe some other application).
		return win32.RECT{}, false
	}
	info, ok := win32.GetGUIThreadInfo(thread)
	if ok && info.HwndCaret != 0 && info.RcCaret.Bottom > info.RcCaret.Top {
		topLeft := win32.POINT{X: info.RcCaret.Left, Y: info.RcCaret.Top}
		bottomRight := win32.POINT{X: info.RcCaret.Right, Y: info.RcCaret.Bottom}
		if win32.ClientToScreen(info.HwndCaret, &topLeft) && win32.ClientToScreen(info.HwndCaret, &bottomRight) {
			return win32.RECT{Left: topLeft.X, Top: topLeft.Y, Right: bottomRight.X, Bottom: bottomRight.Y}, true
		}
	}

	target := window
	if ok && info.HwndFocus != 0 {
		target = info.HwndFocus
	}
	reply := make(chan caretReply, 1)
	select {
	case c.requests <- caretRequest{window: target, reply: reply}:
	default:
		// A previous query is still stuck in an unresponsive application.
		return win32.RECT{}, false
	}
	select {
	case answer := <-reply:
		if answer.ok && answer.rect.Bottom > answer.rect.Top {
			return answer.rect, true
		}
	case <-time.After(150 * time.Millisecond):
	}
	return win32.RECT{}, false
}
