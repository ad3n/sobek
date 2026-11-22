package parser

import (
	"fmt"
	"sort"

	"github.com/ad3n/sobek/file"
	"github.com/ad3n/sobek/token"
)

const (
	err_UnexpectedToken      = "Unexpected token %v"
	err_UnexpectedEndOfInput = "Unexpected end of input"
	err_UnexpectedEscape     = "Unexpected escape"
)

// An Error represents a parsing error. It includes the position where the error occurred and a message/description.
type Error struct {
	File    *file.File
	Message string
	Offset  int
}

func (self Error) Error() string {
	filename := self.File.Name()
	if filename == "" {
		filename = "(anonymous)"
	}
	pos := self.File.Position(self.Offset)
	return fmt.Sprintf("%s: Line %d:%d %s",
		filename,
		pos.Line,
		pos.Column,
		self.Message,
	)
}

func (self *_parser) error(place any, msg string, msgValues ...any) *Error {
	idx := file.Idx(0)
	switch place := place.(type) {
	case int:
		idx = self.idxOf(place)
	case file.Idx:
		if place == 0 {
			idx = self.idxOf(self.chrOffset)
		} else {
			idx = place
		}
	default:
		panic(fmt.Errorf("error(%T, ...)", place))
	}

	msg = fmt.Sprintf(msg, msgValues...)
	e := &Error{
		Message: msg,
		File:    self.file,
		Offset:  int(idx) - self.base,
	}
	self.errors.Add(e)
	return e
}

func (self *_parser) errorUnexpected(idx file.Idx, chr rune) error {
	if chr == -1 {
		return self.error(idx, err_UnexpectedEndOfInput)
	}
	return self.error(idx, err_UnexpectedToken, token.ILLEGAL)
}

func (self *_parser) errorUnexpectedToken(tkn token.Token) error {
	switch tkn {
	case token.EOF:
		return self.error(file.Idx(0), err_UnexpectedEndOfInput)
	}
	value := tkn.String()
	switch tkn {
	case token.BOOLEAN, token.NULL:
		value = self.literal
	case token.IDENTIFIER:
		return self.error(self.idx, "Unexpected identifier")
	case token.KEYWORD:
		// TODO Might be a future reserved word
		return self.error(self.idx, "Unexpected reserved word")
	case token.ESCAPED_RESERVED_WORD:
		return self.error(self.idx, "Keyword must not contain escaped characters")
	case token.NUMBER:
		return self.error(self.idx, "Unexpected number")
	case token.STRING:
		return self.error(self.idx, "Unexpected string")
	}
	return self.error(self.idx, err_UnexpectedToken, value)
}

// ErrorList is a list of *Errors.
type ErrorList []*Error

// Add adds an Error with given position and message to an ErrorList.
func (self *ErrorList) Add(e *Error) {
	*self = append(*self, e)
}

// Reset resets an ErrorList to no errors.
func (self *ErrorList) Reset() { *self = (*self)[0:0] }

func (self ErrorList) Len() int      { return len(self) }
func (self ErrorList) Swap(i, j int) { self[i], self[j] = self[j], self[i] }
func (self ErrorList) Less(i, j int) bool {
	x := self[i]
	y := self[j]
	xName := x.File.Name()
	yName := y.File.Name()
	if xName < yName {
		return true
	}
	if xName == yName {
		return x.Offset < y.Offset
	}
	return false
}

func (self ErrorList) Sort() {
	sort.Sort(self)
}

// Error implements the Error interface.
func (self ErrorList) Error() string {
	switch len(self) {
	case 0:
		return "no errors"
	case 1:
		return self[0].Error()
	}
	return fmt.Sprintf("%s (and %d more errors)", self[0].Error(), len(self)-1)
}

// Err returns an error equivalent to this ErrorList.
// If the list is empty, Err returns nil.
func (self ErrorList) Err() error {
	if len(self) == 0 {
		return nil
	}
	return self
}
