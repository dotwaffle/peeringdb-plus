package graph

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// MarshalGlobalID encodes a type name and integer ID into a Relay-compliant opaque global ID.
// Format: base64("TypeName:IntegerID")
func MarshalGlobalID(typeName string, id int) string {
	return base64.StdEncoding.EncodeToString(
		[]byte(fmt.Sprintf("%s:%d", typeName, id)),
	)
}

// UnmarshalGlobalID decodes a Relay global ID back to type name and integer ID.
func UnmarshalGlobalID(globalID string) (typeName string, id int, err error) {
	decoded, err := base64.StdEncoding.DecodeString(globalID)
	if err != nil {
		return "", 0, fmt.Errorf("decode global ID: %w", err)
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid global ID format: expected TypeName:ID")
	}
	id, err = strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid ID in global ID %q: %w", parts[1], err)
	}
	return parts[0], id, nil
}
