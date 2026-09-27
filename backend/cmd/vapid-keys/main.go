// Command vapid-keys prints a new VAPID key pair for web push:
//
//	go run ./cmd/vapid-keys
//
// Put the private key in the backend .env as WEB_PUSH_VAPID_PRIVATE_KEY.
// The public key is served to browsers by the API automatically.
package main

import (
	"fmt"
	"log"

	"github.com/filmorauz/backend/services"
)

func main() {
	priv, pub, err := services.GenerateVAPIDKeys()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("WEB_PUSH_VAPID_PRIVATE_KEY=%s\n# public key (for reference): %s\n", priv, pub)
}
