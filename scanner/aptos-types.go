package scanner

import "encoding/hex"

// ToHex returns the hex representation of b, prefixed with '0x'.
func ToHex(b []byte) string {
        enc := make([]byte, len(b)*2+2)
        copy(enc, "0x")
        hex.Encode(enc[2:], b)
        return string(enc)
}
