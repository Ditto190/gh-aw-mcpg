// Package mountspec parses container bind-mount declarations.
package mountspec

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Spec is a parsed bind-mount declaration.
type Spec struct {
	Source      string
	Destination string
	Writable    bool
}

// ErrorKind identifies the invalid part of a mount declaration.
type ErrorKind int

const (
	InvalidFormat ErrorKind = iota
	EmptySource
	EmptyDestination
	RelativeSource
	RelativeDestination
	InvalidOptions
)

// ParseError describes an invalid mount declaration.
type ParseError struct {
	Kind ErrorKind
}

func (e *ParseError) Error() string {
	switch e.Kind {
	case InvalidFormat:
		return "expected 'source:dest:mode'"
	case EmptySource, EmptyDestination:
		return "source and destination must not be empty"
	case RelativeSource:
		return "host source must be an absolute path"
	case RelativeDestination:
		return "container destination must be an absolute path"
	default:
		return "invalid mount options"
	}
}

// Parse parses a "source:dest[:mode]" bind-mount declaration. Omitted mode
// defaults to read-write; mode may contain one "ro" or "rw" option.
func Parse(spec string) (Spec, error) {
	parts := strings.Split(spec, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return Spec{}, &ParseError{Kind: InvalidFormat}
	}
	if parts[0] == "" {
		return Spec{}, &ParseError{Kind: EmptySource}
	}
	if parts[1] == "" {
		return Spec{}, &ParseError{Kind: EmptyDestination}
	}
	if !filepath.IsAbs(parts[0]) {
		return Spec{}, &ParseError{Kind: RelativeSource}
	}
	if !filepath.IsAbs(parts[1]) {
		return Spec{}, &ParseError{Kind: RelativeDestination}
	}

	mount := Spec{Source: parts[0], Destination: parts[1], Writable: true}
	if len(parts) == 2 {
		return mount, nil
	}

	modeSet := false
	for _, option := range strings.Split(parts[2], ",") {
		switch option = strings.TrimSpace(option); option {
		case "ro", "rw":
			if modeSet {
				return Spec{}, fmt.Errorf("%w: conflicting mount options", &ParseError{Kind: InvalidOptions})
			}
			modeSet = true
			mount.Writable = option == "rw"
		case "":
			return Spec{}, fmt.Errorf("%w: empty mount option", &ParseError{Kind: InvalidOptions})
		default:
			return Spec{}, fmt.Errorf("%w: unsupported mount option", &ParseError{Kind: InvalidOptions})
		}
	}
	return mount, nil
}

// ParseRequiredMode parses a bind-mount declaration that must explicitly
// include its mode.
func ParseRequiredMode(spec string) (Spec, error) {
	parts := strings.Split(spec, ":")
	if len(parts) != 3 {
		return Spec{}, &ParseError{Kind: InvalidFormat}
	}
	if parts[2] != "ro" && parts[2] != "rw" {
		return Spec{}, &ParseError{Kind: InvalidOptions}
	}
	return Parse(spec)
}
