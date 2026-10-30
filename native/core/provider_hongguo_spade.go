// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"math/bits"
	"strings"
)

func hongguoAppContentKey(value string) ([]byte, error) {
	if key, err := hongguoContentKey(value); err == nil {
		return key, nil
	}
	if len(value) > 1024 {
		return nil, errors.New("红果媒体密钥数据过长")
	}
	raw, err := decodeHongguoBase64(value)
	if err != nil || len(raw) < 3 {
		return nil, errors.New("红果媒体密钥编码无效")
	}
	length := min(len(raw)-int(raw[0]^raw[1]^raw[2])+47, len(raw)-1)
	if length < 33 {
		return nil, errors.New("红果媒体密钥长度无效")
	}
	work := append([]byte(nil), raw[1:1+length]...)
	previousEven, previousOdd := byte(246), byte(85)
	for index, current := range work {
		previous := previousEven
		if index%2 == 0 {
			previousEven = current
		} else {
			previous = previousOdd
			previousOdd = current
		}
		work[index] = byte(int(previous^current) - 21 - bits.OnesCount(uint(index)))
	}
	key, err := hex.DecodeString(string(work[1:33]))
	if err != nil || len(key) != aes.BlockSize {
		return nil, errors.New("红果媒体密钥内容无效")
	}
	return key, nil
}

var hongguoSpadeConstants = [64]byte{
	0x4D, 0xD4, 0xC2, 0xE6, 0xB8, 0x31, 0x62, 0x09, 0x0E, 0x52, 0xB3, 0xC7, 0xA6, 0x73, 0x3B, 0xA4,
	0x1C, 0xB2, 0x46, 0x2B, 0x82, 0x9A, 0xB5, 0x8A, 0x19, 0x6B, 0x39, 0xDB, 0x57, 0x17, 0x75, 0x24,
	0xF4, 0x9B, 0xAF, 0x7F, 0x08, 0xE8, 0xD6, 0x8D, 0x26, 0xA7, 0x2E, 0x37, 0xC1, 0xA9, 0x5A, 0x2F,
	0x1F, 0x05, 0xA5, 0x18, 0x92, 0xAE, 0xF2, 0x94, 0x97, 0x32, 0xB6, 0x2A, 0x38, 0xAA, 0xDD, 0x58,
}

func decryptHongguoSpadeURL(spade string, keySeed []byte) (string, error) {
	raw, err := decodeHongguoBase64(spade)
	if err != nil {
		return "", errors.New("红果加密地址编码无效")
	}
	if len(raw) < 5 || raw[0] != 0xA8 || raw[2] != 0x01 || raw[3] != 0x00 {
		return "", errors.New("红果加密地址结构无效")
	}
	cipherData := raw[4:]
	cipherData = cipherData[:(len(cipherData)/aes.BlockSize)*aes.BlockSize]
	if len(cipherData) == 0 {
		return "", errors.New("红果加密地址长度不足")
	}
	if len(keySeed) == 0 {
		return "", errors.New("红果加密地址缺少解密种子")
	}

	first := sha512.Sum512(keySeed)
	material := make([]byte, 0, len(first)+len(hongguoSpadeConstants))
	material = append(material, first[:]...)
	material = append(material, hongguoSpadeConstants[:]...)
	second := sha512.Sum512(material)
	block, err := aes.NewCipher(second[:16])
	if err != nil {
		return "", errors.New("红果加密地址密钥无效")
	}
	plaintext := make([]byte, len(cipherData))
	cipher.NewCBCDecrypter(block, second[16:32]).CryptBlocks(plaintext, cipherData)
	if length := len(plaintext); length > 0 {
		if pad := int(plaintext[length-1]); pad >= 1 && pad <= aes.BlockSize && pad <= length {
			valid := true
			for _, value := range plaintext[length-pad:] {
				if int(value) != pad {
					valid = false
					break
				}
			}
			if valid {
				plaintext = plaintext[:length-pad]
			}
		}
	}
	address := string(bytes.TrimRight(plaintext, "\x00"))
	if address == "" {
		return "", errors.New("红果加密地址解密结果为空")
	}
	return address, nil
}

func hongguoSpadeKeySeed(videoData map[string]any) ([]byte, error) {
	encoded := mapString(videoData, "key_seed")
	if strings.TrimSpace(encoded) == "" {
		return nil, errors.New("红果取流响应缺少 key_seed")
	}
	raw, err := decodeHongguoBase64(encoded)
	if err != nil || len(raw) == 0 {
		return nil, errors.New("红果 key_seed 编码无效")
	}
	return raw, nil
}
