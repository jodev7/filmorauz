package services

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"
)

// RFC 8291 Appendix A.
func TestEncryptPayloadRFC8291Vector(t *testing.T) {
	asPrivBytes, _ := decodeB64("yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw")
	asPriv, err := ecdh.P256().NewPrivateKey(asPrivBytes)
	if err != nil {
		t.Fatal(err)
	}
	salt, _ := decodeB64("DGv6ra1nlYgDCS1FRnbzlw")
	var sub PushSubscription
	sub.Keys.P256dh = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	sub.Keys.Auth = "BTBZMqHH6r4Tts7J_aSIgg"
	out, err := encryptPayload([]byte("When I grow up, I want to be a watermelon"), sub, salt, asPriv)
	if err != nil {
		t.Fatal(err)
	}
	want := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
	if got := b64(out); got != want {
		t.Fatalf("ciphertext mismatch\n got %s\nwant %s", got, want)
	}
}

func TestVAPIDJWT(t *testing.T) {
	priv, pub, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	keys, err := ParseVAPIDPrivateKey(priv, "mailto:a@b.c")
	if err != nil {
		t.Fatal(err)
	}
	if keys.PublicKey != pub {
		t.Fatalf("derived public key mismatch")
	}
	tok, err := keys.vapidJWT("https://fcm.googleapis.com/fcm/send/abc", time.Unix(1_700_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("bad jwt %q", tok)
	}
	claimsRaw, _ := decodeB64(parts[1])
	var claims map[string]interface{}
	_ = json.Unmarshal(claimsRaw, &claims)
	if claims["aud"] != "https://fcm.googleapis.com" || claims["sub"] != "mailto:a@b.c" {
		t.Fatalf("claims %v", claims)
	}
	sig, _ := decodeB64(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(&keys.private.PublicKey, sum[:], r, s) {
		t.Fatal("signature does not verify")
	}
}
