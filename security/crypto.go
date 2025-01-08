package security

import (
	"fmt"
	"strings"
	"errors"
	"crypto/aes"
	"crypto/md5"
	"crypto/cipher"
	"crypto/sha512"
	"crypto/rand"
	"time"
	"bytes"
	"encoding/binary"
	"encoding/base64"
	"encoding/hex"
	"golang.org/x/crypto/argon2"
	"cogged/log"
)

func B64Encode(input []byte) string {
	output := base64.RawURLEncoding.EncodeToString(input)
	return output
}

func B64Decode(input string) []byte {
	output, err := base64.RawURLEncoding.DecodeString(input)
	if err != nil { log.Error("base64 decode",err) }
	return output
}

func GenerateRandomBytes(length int) ([]byte, error) {
	randomBytes := make([]byte, length)
	_, err := rand.Read(randomBytes)
	return randomBytes, err
}

func GenerateGuid() (string, error) {

	randomBytes, err := GenerateRandomBytes(16)

	if len(randomBytes) >= 5 {
		randomBytes[6] &= 0b00001111 // Clear the first four bits
		randomBytes[6] |= 0b01000000 // Set the second bit to 1 to indicate UUID version 4
		randomBytes[8] &= 0b00111111 // Clear the first two bits
		randomBytes[8] |= 0b10000000 // Set the first bit to 1
	}

	hexString := hex.EncodeToString(randomBytes)

	// 8-4-4-4-12 format
	guidString := hexString[0:8] + "-" + hexString[8:12] + "-" + hexString[12:16] + "-" + hexString[16:20] + "-" + hexString[20:32]

	return guidString, err
}

func GenerateSgi() string {

	tNowMsec := time.Now().UnixNano() / 1e6
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.LittleEndian, tNowMsec)
	var bytestr string
	if err == nil {
		byteArray := buf.Bytes()

		// Extract the lower 6 bytes of the timestamp byte array value
		lower6Bytes := byteArray[:6]
		rand4bytes,_ := GenerateRandomBytes(4)
		finalbytes := append([]byte(lower6Bytes),rand4bytes...)
		bytestr = B64Encode(finalbytes)

	} else {
		rand10bytes,_ := GenerateRandomBytes(10)
		bytestr = B64Encode(rand10bytes)
	}

	return bytestr
}

func MD5SumHex(data []byte) string {
	return fmt.Sprintf("%032x", md5.Sum(data))
}

func SHA512Hash(data []byte) []byte {
	sum := sha512.Sum512_256(data) // returns 32 bytes (256bits)
	return sum[:]
}

func Argon2IDKey(password string, saltBytes []byte) []byte {
	hashBytes := argon2.IDKey([]byte(password), saltBytes, 1, 32*1024, 1, 32)
	return hashBytes
}

func HashPassword(password string, saltBytes []byte) string {
	hashBytes := Argon2IDKey(password, saltBytes)
	saltStr := B64Encode(saltBytes)
	hashStr := B64Encode(hashBytes)
	returnStr := hashStr + "$" + saltStr
	return returnStr // <b64hash>$<b64salt>
}

func GeneratePasswordHash(password string) string {
	saltBytes,_ := GenerateRandomBytes(16)
	return HashPassword(password, saltBytes) // <b64hash>$<b64salt>
}

// hash arg should be a string of the format: <b64hash>$<b64salt>
func VerifyPasswordHash(storedHash, password string) bool {
	hashParts := strings.Split(storedHash,"$")

	if len(hashParts) < 2 {
		return false
	}
	tryHash := HashPassword(password,B64Decode(hashParts[1]))
	return tryHash == storedHash
}


// output format: <b64ciphertext_plus_aad>.<b64nonce>
func AESGCMEncrypt(keyB64Str, plainText string) (string, error) {
	block, err := aes.NewCipher(B64Decode(keyB64Str))
	if err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceBytes,_ := GenerateRandomBytes(12)
	cipherText := aesgcm.Seal(nil, nonceBytes, []byte(plainText), nil)

	return B64Encode(cipherText) + "." + B64Encode(nonceBytes), nil
}


// ciphertext input format: <b64ciphertext_plus_aad>.<b64nonce>
func AESGCMDecrypt(keyB64Str, cipherText string) (string, error) {
	block, err := aes.NewCipher(B64Decode(keyB64Str))
	if err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	cipherTextParts := strings.Split(cipherText,".")

	if len(cipherTextParts) < 2 {
		return "", errors.New("invalid ciphertext string")
	}
	cipherTextBytes := B64Decode(cipherTextParts[0])
	nonceBytes := B64Decode(cipherTextParts[1])

	plainText, err := aesgcm.Open(nil, nonceBytes, cipherTextBytes, nil)
	if err != nil {
		return "", err
	}

	return string(plainText), nil
}

