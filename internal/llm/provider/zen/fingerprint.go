// Package zen implements the client fingerprint required by OpenCode Zen's free tier
// (https://opencode.ai/zen/v1).
//
// The free tier is not an open anonymous endpoint: the gateway rejects any request that does
// not carry the fingerprint an official OpenCode client sends. All of the following are
// required (missing any one yields 403 FreeTierError, an old version yields 426):
//
//   - User-Agent:      "opencode/<version>", lowercase, version >= 1.18
//   - x-opencode-session: "ses_" + 12 lowercase hex + 14 base62 chars
//   - body "stream":   true
//   - body "tools":    must contain the names bash, glob, grep and read
//   - Authorization:   "Bearer public" (not validated, sent for parity)
//
// The session id is OpenCode's ordinary descending identifier: the hex field encodes
// ~(Date.now()*0x1000 + counter) truncated to 48 bits, and the gate requires the low 12 bits
// to decode to a counter in [0,63]. The rest is not validated (timestamp magnitude is not
// checked). See https://github.com/jcleng/opencode-free-provider for the live ablation.
package zen

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"
)

// UserAgent is the client identity advertised to the free-tier gate. Any "opencode/<n>"
// with n >= 18 is accepted; the value here is stable so the fingerprint does not depend on
// our own release number.
const UserAgent = "opencode/1.18.0"

// AuthBearerValue is sent as "Authorization: Bearer <value>" on free-tier requests. The
// gateway does not validate it, but its absence breaks parity with the official client.
const AuthBearerValue = "public"

// RequiredToolNames are the tool names the free-tier body must advertise. A request missing
// any of them is rejected; a duplicate name is also rejected.
var RequiredToolNames = []string{"bash", "glob", "grep", "read"}

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// SessionID returns a fresh free-tier session id of the exact shape the gate accepts:
// "ses_" + 12 lowercase hex + 14 base62 chars.
//
// counter must be in [0,63]; higher values are clamped, since the gate rejects them. A new
// id is generated per request, which keeps the counter well inside the accepted range.
func SessionID(counter uint64) (string, error) {
	if counter > 63 {
		counter = 63
	}
	nowMs := uint64(time.Now().UnixMilli())

	// ~(Date.now()*0x1000 + counter), truncated to 48 bits.
	v := ^(nowMs*0x1000 + counter)
	v &= 0xFFFFFFFFFFFF
	hexPart := fmt.Sprintf("%012x", v)

	tail, err := base62(14)
	if err != nil {
		return "", err
	}
	return "ses_" + hexPart + tail, nil
}

// base62 returns n random chars from the base62 alphabet.
func base62(n int) (string, error) {
	out := make([]byte, n)
	max := big.NewInt(int64(len(base62Alphabet)))
	for i := range out {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = base62Alphabet[idx.Int64()]
	}
	return string(out), nil
}
