package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// NewSessionID returns a unique, time-sortable ID such as
// "20261105T120000Z-a1b2c3". It is also used for event IDs.
func NewSessionID(now time.Time) string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return now.UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b)
}

// NewUUID returns an RFC 4122 version 4 UUID.
func NewUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// NextSeqID returns prefix followed by one more than the highest numeric
// suffix among existing IDs with that prefix.
func NextSeqID(prefix string, existing []string) string {
	max := 0
	for _, id := range existing {
		rest, ok := strings.CutPrefix(id, prefix)
		if !ok {
			continue
		}
		if n, err := strconv.Atoi(rest); err == nil && n > max {
			max = n
		}
	}
	return prefix + strconv.Itoa(max+1)
}
