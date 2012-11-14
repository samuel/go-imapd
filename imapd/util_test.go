package imapd

import (
	// "fmt"
	"reflect"
	"testing"
)

func TestParseRangeSet(t *testing.T) {
	exp := []Range{{1, 3, false}}
	if rs := parseRangeSet("1:3"); !reflect.DeepEqual(rs, exp) {
		t.Fatalf("parseRangeSet returned %+v expected %+v", rs, exp)
	}

	exp = []Range{{1, 0, true}}
	if rs := parseRangeSet("1:*"); !reflect.DeepEqual(rs, exp) {
		t.Fatalf("parseRangeSet returned %+v expected %+v", rs, exp)
	}

	exp = []Range{{1, 0, false}, {2, 4, false}, {5, 0, true}}
	if rs := parseRangeSet("1,2:4,5:*"); !reflect.DeepEqual(rs, exp) {
		t.Fatalf("parseRangeSet returned %+v expected %+v", rs, exp)
	}

	exp = nil
	if rs := parseRangeSet("abc"); !reflect.DeepEqual(rs, exp) {
		t.Fatalf("parseRangeSet returned %+v expected %+v", rs, exp)
	}
}

func TestParseDataItemName(t *testing.T) {
	if items, err := parseMessageDataItemNames("UID"); err != nil {
		t.Fatalf("parseMessageDataItemNames returned error: %+v", err)
	} else if items == nil {
		t.Fatalf("parseMessageDataItemNames returned nil items")
	} else {
		exp := []MessageDataItemName{{"UID", "", nil, nil}}
		if !reflect.DeepEqual(items, exp) {
			t.Fatalf("parseMessageDataItemNames returned %+v expected %+v", items, exp)
		}
	}

	if items, err := parseMessageDataItemNames("(BODY[] RFC822.TEXT)"); err != nil {
		t.Fatalf("parseMessageDataItemNames returned error: %+v", err)
	} else if items == nil {
		t.Fatalf("parseMessageDataItemNames returned nil items")
	} else {
		exp := []MessageDataItemName{{"BODY[]", "", nil, nil}, {"RFC822.TEXT", "", nil, nil}}
		if !reflect.DeepEqual(items, exp) {
			t.Fatalf("parseMessageDataItemNames returned %+v expected %+v", items, exp)
		}
	}

	if items, err := parseMessageDataItemNames("(BODY.PEEK[HEADER.FIELDS (DATE FROM)]<5.20>)"); err != nil {
		t.Fatalf("parseMessageDataItemNames returned error: %+v", err)
	} else if items == nil {
		t.Fatalf("parseMessageDataItemNames returned nil items")
	} else {
		exp := []MessageDataItemName{{"BODY.PEEK[]", "HEADER.FIELDS", []string{"DATE", "FROM"}, []int{5, 20}}}
		if !reflect.DeepEqual(items, exp) {
			t.Fatalf("parseMessageDataItemNames returned %+v expected %+v", items, exp)
		}
	}

	if _, err := parseMessageDataItemNames("(UID INVALID)"); err == nil {
		t.Fatalf("parseMessageDataItemNames returned nil error on invalid input")
	}
}
