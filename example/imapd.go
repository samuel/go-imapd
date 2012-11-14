package main

import (
	// "fmt"
	"crypto/tls"
	"log"
	"strings"
	"time"

	"github.com/samuel/go-imapd/imapd"
)

type TestMailbox struct {
}

func (mb *TestMailbox) Info() (imapd.MailboxInfo, error) {
	return imapd.MailboxInfo{
		NextUid:     2,
		UidValidity: 3,
		Exists:      0,
		Recent:      0,
	}, nil
}

// BODY.PEEK[HEADER.FIELDS (Date Subject From Sender Reply-To To Cc Message-ID References In-Reply-To)] INTERNALDATE

func (mb *TestMailbox) FetchMessagesByUID(ranges []imapd.Range, items []imapd.MessageDataItemName) (map[uint32][]imapd.MessageDataItem, error) {
	res := make(map[uint32][]imapd.MessageDataItem)
	data := make([]imapd.MessageDataItem, 0, len(items))
	for i, it := range items {
		var d interface{} = nil
		switch it.Name {
		case "UID":
			d = i + 1
		case "FLAGS":
			d = []string{imapd.FlagSeen}
		case "INTERNALDATE":
			d = time.Now()
		case "BODY.PEEK[]", "BODY[]":
			it.Name = "BODY[]"
			d = []byte(
				"Date: Sat, 12 Jun 2012 08:09:48 -0400 (EDT)\r\n" +
					"From: blah@blah.com\r\n" +
					"To: test@examples.com\r\n" +
					"Message-ID: <23913265.72143.7367657838142.JavaMail.cfusion@www4.example.com>\r\n" +
					"Subject: Some email\r\n\r\n")
		}
		if d != nil {
			data = append(data, imapd.MessageDataItem{
				Item: it,
				Data: d,
			})
		}
	}
	for _, r := range ranges {
		res[r.Start] = data
	}
	return res, nil
}

type TestBackend struct {
}

func (b *TestBackend) Mailbox(name string) (imapd.Mailbox, error) {
	if strings.ToLower(name) == "inbox" {
		return &TestMailbox{}, nil
	}
	return nil, imapd.ErrUnknownMailbox
}

func main() {
	cert, err := tls.LoadX509KeyPair("cert.pem", "key.pem")
	if err != nil {
		panic(err)
	}

	im := &imapd.Server{
		Addr:          ":1143",
		InsecureLogin: true,
		Backend:       &TestBackend{},
		TlsConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.VerifyClientCertIfGiven,
			ServerName:   "example.com",
		},
	}
	log.Fatal(im.ListenAndServeTLS())
}
