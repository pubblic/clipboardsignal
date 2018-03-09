// +build windows

package clipboardsignal

import (
	"log"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/akavel/winq"
)

var sighdr = struct {
	sigs map[chan<- Notification]bool
	sync.Mutex
}{
	sigs: make(map[chan<- Notification]bool),
}

var initlock sync.Mutex
var initerr error
var window uintptr

func init() {
	// locked for initialization
	initlock.Lock()
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
		initlock.Unlock()
		return
	}
	defer try.F("DestroyWindow", window)

	try.F("ShowWindow", window, SW_HIDE)

	try.N("AddClipboardFormatListener", window)
	if try.Err != nil {
		initerr = try.Err
		// initialzation has finished.
		initlock.Unlock()
		return
	} else {
		// initialzation has finished.
		initlock.Unlock()
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
	n.Text, n.Err = readText(window)

	sighdr.Lock()
	for out := range sighdr.sigs {
		select {
		case out <- n:
		default:
		}
	}
	sighdr.Unlock()
}

func readText(window uintptr) (string, error) {
	const (
		CF_UNICODETEXT = 13
		GMEM_FIXED     = 0x0000
	)

	var try winq.Try
	try.N("OpenClipboard", window)
	if try.Err != nil {
		return "", try.Err
	}
	defer try.F("CloseClipboard")

	h := try.N("GetClipboardData", CF_UNICODETEXT)
	if try.Err != nil {
		return "", try.Err
	}

	m := try.N("GlobalLock", h)
	if try.Err != nil {
		return "", try.Err
	}

	text := syscall.UTF16ToString((*[1 << 31]uint16)(unsafe.Pointer(m))[:])

	try.N("GlobalUnlock", h)
	if try.Err != nil {
		return "", try.Err
	}

	return text, nil
}

// Notify registers sigc to be notified when clipboard has been changed.
func Notify(sigc chan<- Notification) {
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
	initlock.Lock()
	err := initerr
	initlock.Unlock()
	return err
}

// TODO(pb): MISSING
func ReadSlice() ([]byte, error) // return bytes

// TODO(pb): MISSING
func ReadString() (string, error) // return string

// TODO(pb): MISSING
func WriteSlice([]byte) error

// TODO(pb): MISSING
func WriteString(string) error
