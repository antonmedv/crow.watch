package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
)

func main() {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	token := hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(token))
	fmt.Printf("Token:      %s\n", token)
	fmt.Printf("TOKEN_HASH: %s\n", hex.EncodeToString(hash[:]))
}
