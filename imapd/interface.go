package imapd

import (
	"errors"
)

var (
	ErrUnknownMailbox = errors.New("imapd: no such mailbox")
)

type MailboxResponse struct {
	Name      string
	Delimiter string // hierarchy delimiter

	// It is not possible for any child levels of hierarchy to exist
	// under this name; no child levels exist now and none can be
	// created in the future.
	Noinferiors bool
	// It is not possible to use this name as a selectable mailbox.
	Noselect bool
	// The mailbox has been marked "interesting" by the server; the
	// mailbox probably contains messages that have been added since
	// the last time the mailbox was selected.
	Marked bool
}

type MailboxInfo struct {
	NextUid     uint32
	UidValidity uint32
	Exists      uint32
	Recent      uint32
	// Flags []string
}

type MessageDataItem struct {
	Item MessageDataItemName
	Data interface{}
}

type Mailbox interface {
	Info() (MailboxInfo, error)
	FetchMessagesByUID([]Range, []MessageDataItemName) (map[uint32][]MessageDataItem, error)
}

type Backend interface {
	// ListMailboxes(reference, mailbox string) ([]*MailboxResponse, error)
	Mailbox(name string) (Mailbox, error)
}
