package imapd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	reDataItemName = regexp.MustCompile(`(?i)((body(?:\.peek)?)\[([a-z0-9\.]*)(\s\([a-z\-\s]*\))?\](<\d+.\d+>)?|[a-z0-9\.]+)(\s|$)`)

	validDataItemNames = map[string]bool{
		"BODY":          true,
		"BODY[]":        true,
		"BODY.PEEK[]":   true,
		"BODYSTRUCTURE": true,
		"ENVELOPE":      true,
		"FLAGS":         true,
		"INTERNALDATE":  true,
		"RFC822":        true,
		"RFC822.HEADER": true,
		"RFC822.SIZE":   true,
		"RFC822.TEXT":   true,
		"UID":           true,
	}

	macroMessageDataItemNames = map[string][]MessageDataItemName{
		"FAST": {{Name: "FLAGS"}, {Name: "INTERNALDATE"}, {Name: "RFC822.SIZE"}},
		"ALL":  {{Name: "FLAGS"}, {Name: "INTERNALDATE"}, {Name: "RFC822.SIZE"}, {Name: "ENVELOPE"}},
		"FULL": {{Name: "FLAGS"}, {Name: "INTERNALDATE"}, {Name: "RFC822.SIZE"}, {Name: "ENVELOPE"}, {Name: "BODY"}},
	}
)

type ErrInvalidDataItem string

func (e ErrInvalidDataItem) Error() string {
	return fmt.Sprintf("imapd: '%s' is not a valid data item selector", string(e))
}

type Range struct {
	Start    uint32
	End      uint32
	Infinite bool
}

type MessageDataItemName struct {
	Name       string
	Section    string
	FieldNames []string
	Partial    []int // Two item list: [start, count]
}

func (d MessageDataItemName) String() string {
	out := []string{d.Name}
	if d.Section != "" {
		out[0] = out[0][:len(out[0])-2]
		out = append(out, "[", d.Section)
		if d.FieldNames != nil {
			out = append(out, " (", strings.Join(d.FieldNames, " "), ")")
		}
		out = append(out, "]")
		if d.Partial != nil && len(d.Partial) == 2 {
			out = append(out, "<", strconv.Itoa(d.Partial[0]), ".", strconv.Itoa(d.Partial[1]), ">")
		}
	}
	return strings.Join(out, "")
}

func (r Range) String() string {
	if r.End == 0 {
		if r.Infinite {
			return fmt.Sprintf("%d:*", r.Start)
		}
		return strconv.FormatUint(uint64(r.Start), 10)
	}
	return fmt.Sprintf("%d:%d", r.Start, r.End)
}

// Parse strings of the type: 1,2:5,3:*
func parseRangeSet(rs string) []Range {
	set := make([]Range, 0)
	for _, s := range strings.Split(rs, ",") {
		if len(s) == 0 {
			return nil
		}
		p := strings.Split(s, ":")
		if len(p) > 2 {
			return nil
		}
		start, err := strconv.ParseUint(p[0], 10, 32)
		if err != nil {
			return nil
		}
		end := uint64(0)
		infinite := false
		if len(p) > 1 {
			if p[1] == "*" {
				infinite = true
			} else {
				end, err = strconv.ParseUint(p[1], 10, 32)
				if err != nil {
					return nil
				}
			}
		}
		set = append(set, Range{uint32(start), uint32(end), infinite})
	}
	return set
}

// Parse and validate a data item list: (UID BODY[HEADER.FIELDS (DATE FROM)]<0.1024>)
func parseMessageDataItemNames(names string) ([]MessageDataItemName, error) {
	if names[0] != '(' {
		items := macroMessageDataItemNames[names]
		if items != nil {
			return items, nil
		}
	} else {
		names = names[1 : len(names)-1]
	}

	items := make([]MessageDataItemName, 0)
	for _, s := range reDataItemName.FindAllStringSubmatch(names, -1) {
		item := MessageDataItemName{}
		if s[2] != "" {
			// parse BODY(.PEEK)[... (...)]<X.Y>
			var fieldNames []string = nil
			if s[4] != "" {
				fieldNames = strings.Split(s[4][2:len(s[4])-1], " ")
			}
			var partial []int = nil
			if s[5] != "" {
				p := strings.Split(s[5][1:len(s[5])-1], ".")
				start, err := strconv.Atoi(p[0])
				if err != nil {
					return nil, err
				}
				count, err := strconv.Atoi(p[1])
				if err != nil {
					return nil, err
				}
				partial = []int{start, count}
			}
			item = MessageDataItemName{
				Name:       strings.ToUpper(s[2]) + "[]",
				Section:    strings.ToUpper(s[3]),
				FieldNames: fieldNames,
				Partial:    partial,
			}
		} else {
			item = MessageDataItemName{
				Name: strings.ToUpper(strings.TrimSpace(s[0])),
			}
		}
		if valid := validDataItemNames[item.Name]; !valid {
			return items, ErrInvalidDataItem(s[0])
		}
		items = append(items, item)
	}

	return items, nil
}
