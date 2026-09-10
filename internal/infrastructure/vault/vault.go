// Package vault implementa el almacenamiento seguro del instrumento de pago.
//
// Este adaptador cifra localmente con AES-GCM y sirve para desarrollo y
// pruebas. En producción se sustituye por un HSM o un vault certificado PCI
// (AWS Payment Cryptography, HashiCorp Vault Transit), sin tocar el dominio:
// solo cambia la implementación del puerto.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
)

// ErrKeyTooShort indica una clave de cifrado insuficiente.
var ErrKeyTooShort = errors.New("encryption key must be at least 32 bytes")

// AESVault cifra el instrumento con AES-GCM y lo guarda separado del token.
type AESVault struct {
	gcm     cipher.AEAD
	hmacKey []byte
	mu      sync.RWMutex
	blobs   map[string][]byte
}

// New construye el vault a partir de una clave de al menos 32 bytes.
func New(key string) (*AESVault, error) {
	if len(key) < 32 {
		return nil, ErrKeyTooShort
	}
	suma := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(suma[:])
	if err != nil {
		return nil, fmt.Errorf("cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	hk := sha256.Sum256([]byte("fingerprint|" + key))
	return &AESVault{gcm: gcm, hmacKey: hk[:], blobs: make(map[string][]byte)}, nil
}

// Store cifra el PAN y devuelve la referencia opaca con la que se lo identifica.
// El llamador debe descartar el PAN inmediatamente después.
func (v *AESVault) Store(ref, pan string) error {
	nonce := make([]byte, v.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("nonce: %w", err)
	}
	sellado := v.gcm.Seal(nonce, nonce, []byte(pan), []byte(ref))
	v.mu.Lock()
	defer v.mu.Unlock()
	v.blobs[ref] = sellado
	return nil
}

// Fingerprint deriva una huella estable del PAN con HMAC-SHA256.
//
// Permite detectar que dos tokens corresponden a la misma tarjeta sin
// almacenar ni poder reconstruir el número.
func (v *AESVault) Fingerprint(pan string) string {
	mac := hmac.New(sha256.New, v.hmacKey)
	mac.Write([]byte(pan))
	return hex.EncodeToString(mac.Sum(nil))[:32]
}

// Delete elimina el instrumento cifrado asociado a una referencia.
func (v *AESVault) Delete(ref string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.blobs, ref)
}

// Has indica si existe un instrumento para la referencia.
func (v *AESVault) Has(ref string) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	_, ok := v.blobs[ref]
	return ok
}

// NewReference genera una referencia opaca para el vault.
func NewReference() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
