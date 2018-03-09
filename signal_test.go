// +build windows

package clipboardsignal

import (
	"testing"
	"time"

	"github.com/atotto/clipboard"
)

func TestNotifyStop(t *testing.T) {
	t.Run("TestNotify", testNotify)
	t.Run("TestStop", testStop)
}

func testNotify(t *testing.T) {
	text, err := clipboard.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll fails: %v", err)
	}
	defer clipboard.WriteAll(text)

	c := make(chan Notification, 1)
	Notify(c)
	defer Stop(c)

	const clipboardData = "hello hello hello"
	err = clipboard.WriteAll(clipboardData)
	if err != nil {
		t.Fatalf("WriteAll fails: %v", err)
	}

	n := <-c
	if n.Err != nil {
		t.Fatal(n.Err)
	}
	if n.Text != clipboardData {
		t.Fatalf("the actual clipboard data and expected data do not match; want %q, but got %q", clipboardData, n.Text)
	}
}

func testStop(t *testing.T) {
	text, err := clipboard.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll fails: %v", err)
	}
	defer clipboard.WriteAll(text)

	c := make(chan Notification, 1)

	// stop not registered channel
	Stop(c)

	Notify(c)
	Stop(c)

	// change the content of clipboard
	err = clipboard.WriteAll("blah bula boola")
	if err != nil {
		t.Fatalf("WriteAll fails: %v", err)
	}

	select {
	case <-c:
		t.Fatal("notification should not be delivered; please don't use clipboard for a second")
	case <-time.After(100 * time.Millisecond):
		// Notification han not been delivered within the specified duration.
		// Thus Stop must have worked correctly.
	}
}
