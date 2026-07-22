package auth

import (
	"crypto/md5"
)

// OfflineUUID replicates Java's
//
//	UUID.nameUUIDFromBytes(("OfflinePlayer:" + username).getBytes())
//
// exactly, so offline-mode UUIDs (and therefore skins/behavior tied to them)
// match vanilla launcher conventions.
//
// Java's nameUUIDFromBytes is a name-based (v3, MD5) UUID as defined by
// RFC 4122 section 4.3, EXCEPT that unlike a "proper" RFC 4122 v3 UUID it
// does not use a namespace UUID prefix — it MD5-hashes the name bytes alone.
// The 16-byte MD5 digest then has its version/variant bits patched:
//
//	byte 6: (b6 & 0x0F) | 0x30   -> version 3
//	byte 8: (b8 & 0x3F) | 0x80   -> variant 10xx (RFC 4122 / "IETF")
func OfflineUUID(username string) [16]byte {
	sum := md5.Sum([]byte("OfflinePlayer:" + username))
	sum[6] = (sum[6] & 0x0F) | 0x30
	sum[8] = (sum[8] & 0x3F) | 0x80
	return sum
}

// OfflineUUIDString returns the canonical dashed, lowercase hex form, e.g.
// "5c715f39-0925-3f36-9ac0-25f8dcb1dbd3", matching how Minecraft embeds it
// in launch arguments and in-game APIs.
func OfflineUUIDString(username string) string {
	b := OfflineUUID(username)
	return formatUUID(b)
}

func formatUUID(b [16]byte) string {
	const hexdigits = "0123456789abcdef"
	buf := make([]byte, 36)
	pos := 0
	dashAfter := map[int]bool{4: true, 6: true, 8: true, 10: true}
	for i, v := range b {
		buf[pos] = hexdigits[v>>4]
		buf[pos+1] = hexdigits[v&0x0F]
		pos += 2
		if dashAfter[i+1] {
			buf[pos] = '-'
			pos++
		}
	}
	return string(buf[:pos])
}

// OfflineProfile creates a locally-generated profile: no network calls.
type OfflineProfile struct {
	Username string
	UUID     string // dashed form
}

// NewOfflineProfile builds an OfflineProfile for the given username using
// the vanilla offline UUID derivation.
func NewOfflineProfile(username string) OfflineProfile {
	return OfflineProfile{
		Username: username,
		UUID:     OfflineUUIDString(username),
	}
}
