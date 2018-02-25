// +build windows

package clipboardsignal

import (
	"log"
	"runtime"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/akavel/winq"
)

var signals = make(map[chan<- string]bool)
var mutex sync.Mutex

var wait = make(chan struct{})

func init() {
	winq.Dlls = append(winq.Dlls,
		syscall.MustLoadDLL("msvcrt.dll"),
	)
	go loop()
}

func malloc(n uintptr) uintptr {
	m, _ := new(winq.Try).F("malloc", n)
	if m == 0 {
		panic("out of memory")
	}
	return m
}

func free(m uintptr) {
	new(winq.Try).F("free", m)
}

func newWCHAR(s string) uintptr {
	const SIZEOF_WCHAR = unsafe.Sizeof(uint16(0))
	a := utf16.Encode([]rune(s))
	m := malloc(SIZEOF_WCHAR*uintptr(len(a)) + 1)
	t := m
	for _, u := range a {
		*(*uint16)(unsafe.Pointer(t)) = u
		t += SIZEOF_WCHAR
	}
	*(*uint16)(unsafe.Pointer(t)) = 0
	return m
}

type _UINT uint32
type _LONG int32
type _DWORD uint32
type _POINT struct {
	x _LONG
	y _LONG
}

type _MSG struct {
	window  uintptr
	message _UINT
	wParam  _WPARAM
	lParam  _LPARAM
	time    _DWORD
	pt      _POINT
}

func loop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	const (
		CW_USEDEFAULT      = 0x80000000
		SW_HIDE            = 0
		SW_SHOWDEFAULT     = 10
		WM_CREATE          = 0x0001
		WM_CLIPBOARDUPDATE = 0x031D
	)

	className := newWCHAR("Button")
	windowName := newWCHAR("Void Button")
	defer free(className)
	defer free(windowName)

	var try winq.Try
	window := try.N("CreateWindowEx",
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
		return
	}
	defer try.F("DestroyWindow", window)

	try.F("ShowWindow", window, SW_HIDE)

	try.N("AddClipboardFormatListener", window)
	if try.Err != nil {
		return
	}
	close(wait)
	defer try.F("RemoveClipboardFormatListener", window)

	message := new(_MSG)
	for {
		r, err := try.F("GetMessage",
			message,
			window,
			0,
			0,
			// WM_CLIPBOARDUPDATE,
			// WM_CLIPBOARDUPDATE,
		)
		if r == 0 {
			log.Print("clipboardsignal: window receives WM_QUIT")
			// WM_QUIT
			return
		}
		if int(r) == -1 {
			log.Print("clipboardsignal: GetMessage returned an error:", err)
			return
			// return winq.Error{err, "GetMessage"}
		}
		if message.message == WM_CLIPBOARDUPDATE {
			onUpdate(window)
		}

		try.F("TranslateMessage", message)
		try.F("DispatchMessage", message)
	}
}

func onUpdate(window uintptr) {
	text, _ := readText(window)
	mutex.Lock()
	defer mutex.Unlock()
	for sigc := range signals {
		select {
		case sigc <- text:
		default:
		}
	}
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

func Notify(sigc chan<- string) {
	<-wait

	mutex.Lock()
	defer mutex.Unlock()
	signals[sigc] = true
}

func Stop(sigc chan<- string) {
	<-wait

	mutex.Lock()
	defer mutex.Unlock()
	delete(signals, sigc)
}
