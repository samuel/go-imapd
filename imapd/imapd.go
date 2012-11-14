// Package imtpd implements an IMAP server.
package imapd

// http://tools.ietf.org/html/rfc3501

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPort    = 143
	defaultSSLPort = 993
)

// 2.3.2.  Flags Message Attribute
const (
	FlagSeen     = `\Seen`     // Message has been read
	FlagAnswered = `\Answered` // Message has been answered
	FlagFlagged  = `\Flagged`  // Message is "flagged" for urgent/special attention
	FlagDeleted  = `\Deleted`  // Message is "deleted" for removal by later EXPUNGE
	FlagDraft    = `\Draft`    // Message has not completed composition (marked as a draft)

	// Message is "recently" arrived in this mailbox. This session
	// is the first session to have been notified about this
	// message; if the session is read-write, subsequent sessions
	// will not see \Recent set for this message.  This flag can not
	// be altered by the client.
	//
	// If it is not possible to determine whether or not this
	// session is the first session to be notified about a message,
	// then that message SHOULD be considered recent.
	//
	// If multiple connections have the same mailbox selected
	// simultaneously, it is undefined which of these connections
	// will see newly-arrived messages with \Recent set and which
	// will see it without \Recent set.
	FlagReceent = `\Recent`
)

const (
	internalDateFormat = "02-Jan-2006 15:04:05 -0700"
)

var (
	envelopeFields = []string{
		"Date",
		"Subject",
		"From", // personal name, SMTP at-domain-list (source route), mailbox name, host name
		"Sender",
		"Reply-To",
		"To",
		"CC",
		"BCC",
		"In-Reply-To",
		"Message-Id",
	}
)

type Server struct {
	Addr         string        // TCP address to listen on, ":143" if empty
	ReadTimeout  time.Duration // optional read timeout
	WriteTimeout time.Duration // optional write timeout

	TlsConfig     *tls.Config
	InsecureLogin bool // allow login even when connection isn't secure

	Backend Backend
}

// Connection is implemented by the IMAP library and provided to callers
// customizing their own Servers.
type Connection interface {
	Addr() net.Addr
}

// ListenAndServe listens on the TCP network address srv.Addr and then
// calls Serve to handle requests on incoming connections.
func (srv *Server) ListenAndServe() error {
	addr := srv.Addr
	if addr == "" {
		addr = ":" + strconv.Itoa(defaultPort)
	}
	ln, e := net.Listen("tcp", addr)
	if e != nil {
		return e
	}
	return srv.Serve(ln, false)
}

func (srv *Server) ListenAndServeTLS() error {
	addr := srv.Addr
	if addr == "" {
		addr = ":" + strconv.Itoa(defaultSSLPort)
	}
	if srv.TlsConfig == nil {
		return errors.New("imapd: ListenAndServeTLS called but TlsConfig is nil")
	}
	ln, e := tls.Listen("tcp", addr, srv.TlsConfig)
	if e != nil {
		return e
	}
	return srv.Serve(ln, true)
}

func (srv *Server) Serve(ln net.Listener, secure bool) error {
	defer ln.Close()
	for {
		rw, e := ln.Accept()
		if e != nil {
			if ne, ok := e.(net.Error); ok && ne.Temporary() {
				log.Printf("imapd: Accept error: %v", e)
				continue
			}
			return e
		}
		sess, err := srv.newSession(rw)
		if err != nil {
			continue
		}
		sess.secure = secure
		go sess.serve()
	}
	panic("not reached")
}

type session struct {
	srv           *Server
	rwc           net.Conn
	br            *bufio.Reader
	bw            *bufio.Writer
	secure        bool
	authenticated bool
	mailbox       Mailbox
}

func (srv *Server) newSession(rwc net.Conn) (s *session, err error) {
	s = &session{
		srv: srv,
		rwc: rwc,
		br:  bufio.NewReader(rwc),
		bw:  bufio.NewWriter(rwc),
	}
	return
}

func (s *session) errorf(format string, args ...interface{}) {
	log.Printf("Client error: "+format, args...)
}

func (s *session) sendf(format string, args ...interface{}) error {
	if s.srv.WriteTimeout != 0 {
		s.rwc.SetWriteDeadline(time.Now().Add(s.srv.WriteTimeout))
	}
	fmt.Printf("> '"+format+"'\n", args...)
	if _, err := fmt.Fprintf(s.bw, format, args...); err != nil {
		return err
	}
	return s.bw.Flush()
}

func (s *session) sendlinef(format string, args ...interface{}) error {
	return s.sendf(format+"\r\n", args...)
}

func (s *session) sendobject(obj interface{}) error {
	switch t := obj.(type) {
	case nil:
		s.sendf("NIL")
	case int:
		s.sendf("%d", t)
	case string:
		s.sendf(`"%s"`, strings.Replace(t, `"`, `\"`, -1))
	case []string:
		s.sendf("(%s)", strings.Join(t, " "))
	case time.Time:
		s.sendf(`"` + t.Format(internalDateFormat) + `"`)
	case []byte:
		if err := s.sendf("{%d}\r\n", len(t)); err != nil {
			return err
		}
		if _, err := s.bw.Write(t); err != nil {
			return err
		}
	}
	return errors.New("imapd: unknown type in sendobject")
}

func (s *session) Addr() net.Addr {
	return s.rwc.RemoteAddr()
}

func (s *session) serve() {
	defer s.rwc.Close()
	s.sendlinef("* OK [CAPABILITY IMAP4rev1 AUTH=LOGIN] hostname.com IMAP4rev1 2004.350 at Sun, 8 Aug 2004 13:51:21 -0500 (CDT)")
	for {
		if s.srv.ReadTimeout != 0 {
			s.rwc.SetReadDeadline(time.Now().Add(s.srv.ReadTimeout))
		}
		sl, err := s.br.ReadSlice('\n')
		if err != nil {
			s.errorf("read error: %v", err)
			return
		}

		fmt.Printf("< %s", sl)

		parts := strings.Split(strings.TrimSpace(string(sl)), " ")
		if len(parts) < 2 {
			s.errorf("too short command %s", sl)
			return
		}
		tag, cmd := parts[0], strings.ToLower(parts[1])
		args := []string{}
		if len(parts) > 2 {
			args = parts[2:]
		}

		switch cmd {
		case "noop":
			// TODO: Send any status updates as untagged responses
			s.sendlinef("%s OK NOOP completed", tag)
		case "capability":
			// LITERAL+ IDLE NAMESPACE MAILBOX-REFERRALS BINARY UNSELECT SCAN SORT THREAD=REFERENCES
			// THREAD=ORDEREDSUBJECT MULTIAPPEND SASL-IR LOGIN-REFERRALS AUTH=LOGIN
			caps := []string{"IMAP4rev1"}
			if s.srv.TlsConfig != nil {
				caps = append(caps, "STARTTLS")
			}
			if s.secure || s.srv.InsecureLogin {
				caps = append(caps, "AUTH=login")
			} else {
				caps = append(caps, "LOGINDISABLED")
			}
			s.sendlinef("* CAPABILITY " + strings.Join(caps, " "))
			s.sendlinef("%s OK CAPABILITY completed", tag)
		case "starttls":
			if s.secure {
				s.sendlinef("%s NO connection already secure", tag)
			} else if s.srv.TlsConfig == nil {
				s.sendlinef("%s NO TLS not configured", tag)
			} else {
				s.sendlinef("%s OK Begin TLS negotiation now")
				s.bw.Flush()
				c := tls.Server(s.rwc, s.srv.TlsConfig)
				if err := c.Handshake(); err != nil {
					s.errorf("TLS handshake failed: %+v", err)
					return
				}
				s.rwc = c
				s.br = bufio.NewReader(c)
				s.bw = bufio.NewWriter(c)
				s.secure = true
			}
		// case "authenticate": // 6.2.2
		case "login": // "username" password
			if s.secure || s.srv.InsecureLogin {
				// TODO
				s.authenticated = true
				s.sendlinef("%s OK User logged in", tag)
			} else {
				s.sendlinef("%s NO Login only supported over a secure connection", tag)
			}
		case "close":
			s.sendlinef("%s OK Returned to authenticated state. (Success)", tag)
		case "logout":
			s.sendlinef("* BYE LOGOUT Requested")
			s.sendlinef("%s OK %d good day (Success)", tag, 0)
		case "status": // 6.3.10 - STATUS [mailbox name] ([status data item names])
			if len(args) < 2 {
				s.sendlinef("%s BAD Missing mailbox and item names", tag)
			} else {
				mb, err := s.srv.Backend.Mailbox(args[0])
				if err != nil {
					if err == ErrUnknownMailbox {
						s.sendlinef("%s NO unknown mailbox", tag)
					} else {
						s.errorf("Error selecting mailbox %s: %+v", args[0], err)
						s.sendlinef("%s NO internal error", tag)
					}
				} else {
					info, err := mb.Info()
					if err != nil {
						s.errorf("Error getting info for mailbox %s: %+v", args[0], err)
						s.sendlinef("%s NO internal error", tag)
					} else {
						s.sendf("* STATUS %s (", args[0])
						for i, it := range strings.Split(args[1][1:len(args[1])-1], " ") {
							if i != 0 {
								s.sendf(" ")
							}
							switch strings.ToUpper(it) {
							case "MESSAGES":
								s.sendf("MESSAGES %d", info.Exists)
							case "RECENT":
								s.sendf("RECENT %d", info.Recent)
							case "UIDNEXT":
								s.sendf("UIDNEXT %d", info.NextUid)
							case "UIDVALIDITY":
								s.sendf("UIDVALIDITY %d", info.UidValidity)
							case "UNSEEN":
								// Number of messages which do not have the \Seen flag set.
								s.sendf("UNSEEN %d", info.Unseen)
							}
						}
						s.sendlinef(")")
					}
				}
			}
		case "select": // 6.3.1 - SELECT [mailbox name]
			if len(args) < 1 {
				s.sendlinef("%s BAD Missing mailbox name", tag)
			} else {
				s.mailbox, err = s.srv.Backend.Mailbox(args[0])
				if err != nil {
					if err == ErrUnknownMailbox {
						s.sendlinef("%s NO unknown mailbox", tag)
					} else {
						s.errorf("Error selecting mailbox %s: %+v", args[0], err)
						s.sendlinef("%s NO internal error", tag)
					}
				} else {
					info, err := s.mailbox.Info()
					if err != nil {
						s.errorf("Error getting info for mailbox %s: %+v", args[0], err)
						s.sendlinef("%s NO internal error", tag)
					} else {
						s.sendlinef(`* FLAGS (\Answered \Flagged \Draft \Deleted \Seen)`)
						s.sendlinef(`* OK [PERMANENTFLAGS (\Answered \Flagged \Draft \Deleted \Seen \*)]`)
						s.sendlinef(`* OK [UIDVALIDITY %d]`, info.UidValidity)
						s.sendlinef(`* OK [UIDNEXT %d]`, info.NextUid)
						// s.sendlinef("* OK [UNSEEN %d]", ...) // The message sequence number of the first unseen message in the mailbox.
						s.sendlinef("* %d EXISTS", info.Exists)
						s.sendlinef("* %d RECENT", info.Recent)
						s.sendlinef("%s OK [READ-WRITE] Completed", tag)
					}
				}
			}
		// case "namespace":
		case "list": // 6.3.8: LIST [reference name] [mailbox name with possible wildcards]
			if args[1] == "" {
				s.sendlinef(`* LIST (\Noselect) "/" "/"`)
			} else { // "/"
				s.sendlinef(`* LIST (\HasNoChildren) "/" "INBOX"`)
			}
			s.sendlinef("%s OK LIST complete", tag)
		// case "fetch": // message-set (...)
		// * 1 FETCH (UID 1517)
		// %s OK Success
		case "uid":
			if len(args) < 1 {
				s.sendlinef("%s BAD Missing command", tag)
			} else {
				cmd = strings.ToLower(args[0])
				args = args[1:]
				switch cmd {
				case "fetch": // UID FETCH [uid set] [message data item names or macro]
					// TODO: Check len(args) >= 2
					rangeSet := parseRangeSet(args[0])
					itemNames, err := parseMessageDataItemNames(strings.Join(args[1:], " "))
					if err != nil {
						s.errorf("Error item names %s: %+v", args[0], err)
						s.sendlinef("%s BAD invalid item names", tag)
					} else if rangeSet == nil {
						s.errorf("Error parsing range set %s", args[0])
						s.sendlinef("%s BAD invalid range", tag)
					} else {
						items, err := s.mailbox.FetchMessagesByUID(rangeSet, itemNames)
						if err != nil {
							s.errorf("Error fetching by UID %s %s: %+v", args[0], args[1], err)
							s.sendlinef("%s BAD internal error", tag)
						} else {
							for seqNum, data := range items {
								s.sendf("* %d FETCH (", seqNum)
								for i, v := range data {
									if i == 0 {
										s.sendf("%s ", v.Item.String())
									} else {
										s.sendf(" %s ", v.Item.String())
									}
									s.sendobject(v.Data)
								}
								s.sendlinef(")")
							}
							s.sendlinef("%s OK Success", tag)
						}
					}
				default:
					s.sendlinef("%s BAD Unknown command", tag)
				}
			}
		default:
			s.sendlinef("%s BAD Unknown command", tag)
		}
	}
}
