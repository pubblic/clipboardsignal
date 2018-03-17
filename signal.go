// +build windows

package clipboardsignal

import (
	"log"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/akavel/winq"
	"github.com/pubblic/utf"
)

const (
	_CF_UNICODETEXT = 13
	_GMEM_FIXED     = 0x0000
	_SIZE_UINT16    = unsafe.Sizeof(uint16(0))
	_MAX_SIZE       = 0x80000000
)

var sighdr = struct {
	sigs map[chan<- Notification]bool
	sync.Mutex
}{
	sigs: make(map[chan<- Notification]bool),
}

var uselock sync.Mutex

var initlock = make(chan struct{})
var initerr error
var window uintptr

func init() {
	go loop()
}

type _UINT uint32

type _LONG int32

type _DWORD uint32

type _POINT struct {
	x _LONG
	y _LONG
}

// MSG structure ported for Go
type _MSG struct {
	window  uintptr
	message _UINT
	wParam  _WPARAM
	lParam  _LPARAM
	time    _DWORD
	pt      _POINT
}

func loop() {
	const (
		CW_USEDEFAULT      = 0x80000000
		SW_HIDE            = 0
		SW_SHOWDEFAULT     = 10
		WM_CREATE          = 0x0001
		WM_CLIPBOARDUPDATE = 0x031D
	)

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// TOOD(pb): other class?
	className, _ := syscall.UTF16PtrFromString("Button")
	windowName, _ := syscall.UTF16PtrFromString("Window Button")

	var try winq.Try
	window = try.N("CreateWindowEx",
		0,
		className,
		windowName,
		0,
		CW_USEDEFAULT,
		CW_USEDEFAULT,
		CW_USEDEFAULT,
		CW_USEDEFAULT,
		0,
		0,
		0,
		0,
	)
	if try.Err != nil {
		initerr = try.Err
		close(initlock)
		return
	}
	defer try.F("DestroyWindow", window)

	try.F("ShowWindow", window, SW_HIDE)

	try.N("AddClipboardFormatListener", window)
	if try.Err != nil {
		initerr = try.Err
		// initialzation has finished.
		close(initlock)
		return
	} else {
		// initialzation has finished.
		close(initlock)
	}
	defer try.F("RemoveClipboardFormatListener", window)

	message := new(_MSG)
	for {
		r, err := try.F("GetMessage",
			message,
			window,
			0, // WM_CLIPBOARDUPDATE
			0, // WM_CLIPBOARDUPDATE
		)
		// WM_QUIT message is received
		if r == 0 {
			log.Print("clipboardsignal: window receives WM_QUIT")
			break
		}
		// -1
		if r == 0xFFFF {
			log.Print("clipboardsignal: ", winq.Error{err, "GetMessage"})
			break
		}
		if message.message == WM_CLIPBOARDUPDATE {
			onClipboardUpdate()
		}

		try.F("TranslateMessage", message)
		try.F("DispatchMessage", message)
	}

	runtime.KeepAlive(className)
	runtime.KeepAlive(windowName)
}

type Notification struct {
	Text string
	Err  error
}

func onClipboardUpdate() {
	var n Notification
	n.Text, n.Err = ReadAll()

	sighdr.Lock()
	for out := range sighdr.sigs {
		select {
		case out <- n:
		default:
		}
	}
	sighdr.Unlock()
}

// Notify registers sigc to be notified when clipboard has been changed.
func Notify(sigc chan<- Notification) {
	// <-initlock
	sighdr.Lock()
	sighdr.sigs[sigc] = true
	sighdr.Unlock()
}

// Stop stops for sigc to be notified.
func Stop(sigc chan<- Notification) {
	sighdr.Lock()
	delete(sighdr.sigs, sigc)
	sighdr.Unlock()
}

// Wait waits for initialization to finish.
func Wait() error {
	<-initlock
	return initerr
}

// ReadAll reads the content of the clipboard. Compatible with github.com/atotto/clipboard.
func ReadAll() (string, error) {
	<-initlock
	if initerr != nil {
		return "", initerr
	}

	uselock.Lock()
	defer uselock.Unlock()

	runtime.LockOSThread()
	defer runtime.LockOSThread()

	var try winq.Try
	try.N("OpenClipboard", window)
	if try.Err != nil {
		return "", try.Err
	}
	defer try.F("CloseClipboard")

	h := try.N("GetClipboardData", _CF_UNICODETEXT)
	if try.Err != nil {
		return "", try.Err
	}

	m := try.N("GlobalLock", h)
	if try.Err != nil {
		return "", try.Err
	}

	u := (*[_MAX_SIZE]uint16)(unsafe.Pointer(m))[:]
	for i, r := range u {
		if r == 0 {
			u = u[:i]
			break
		}
	}
	s := utf.UTF8DecodeToString(u)

	try.N("GlobalUnlock", h)
	if try.Err != nil {
		return "", try.Err
	}

	return s, nil
}

// WriteAll writes s into the clipboard. Compatible with github.com/atotto/clipboard.
func WriteAll(s string) error {
	<-initlock
	if initerr != nil {
		return initerr
	}

	// issue(pb): CloseClipboard affects all threads.
	uselock.Lock()
	defer uselock.Unlock()

	// issue(pb): functions operating on a open clipboard may be
	// executed on a different thread. this is an error.
	//
	// ERROR_CLIPBOARD_NOT_OPEN (0x58A)
	// https://msdn.microsoft.com/en-us/library/ms838437.aspx
	runtime.LockOSThread()
	defer runtime.LockOSThread()

	var try winq.Try
	try.N("OpenClipboard", window)
	if try.Err != nil {
		return try.Err
	}
	defer try.F("CloseClipboard")

	try.N("EmptyClipboard")
	if try.Err != nil {
		return try.Err
	}

	n := utf.UTF16CountInString(s)
	n++ // terminating NULL

	h := try.N("GlobalAlloc", _GMEM_FIXED, uintptr(n)*_SIZE_UINT16)
	if try.Err != nil {
		return try.Err
	}

	l := try.N("GlobalLock", h)
	if try.Err != nil {
		return try.Err
	}

	dst := (*[_MAX_SIZE]uint16)(unsafe.Pointer(l))[:n]
	utf.UTF16EncodeString(dst, s)
	dst[n-1] = 0

	try.N("GlobalUnlock", h)
	if try.Err != nil {
		return try.Err
	}

	try.N("SetClipboardData", _CF_UNICODETEXT, h)
	return try.Err
}

func Write(p []byte) error {
	<-initlock
	if initerr != nil {
		return initerr
	}

	uselock.Lock()
	defer uselock.Unlock()

	runtime.LockOSThread()
	defer runtime.LockOSThread()

	var try winq.Try
	try.N("OpenClipboard", window)
	if try.Err != nil {
		return try.Err
	}
	defer try.F("CloseClipboard")

	n := utf.UTF16Count(p)
	n++ // terminating NULL

	h := try.N("GlobalAlloc", _GMEM_FIXED, uintptr(n)*_SIZE_UINT16)
	if try.Err != nil {
		return try.Err
	}

	l := try.N("GlobalLock", h)
	if try.Err != nil {
		return try.Err
	}

	dst := (*[_MAX_SIZE]uint16)(unsafe.Pointer(l))[:n]
	utf.UTF16Encode(dst, p)
	dst[n-1] = 0

	try.N("GlobalUnlock", h)
	if try.Err != nil {
		return try.Err
	}

	try.N("SetClipboardData", _CF_UNICODETEXT, h)
	return try.Err
}
