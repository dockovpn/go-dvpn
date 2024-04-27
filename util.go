/*
 * Copyright (c) 2024. Dockovpn Solutions OÜ
 */

package go_dvpn

import (
	b64 "encoding/base64"
	"encoding/json"
	"strings"
	"unicode"
)

type DvpnContainerOptions struct {
	ImageUrl      string
	ContainerName string
}

type RegistryCreds struct {
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	Email         string `json:"email,omitempty"`
	Serveraddress string `json:"serveraddress,omitempty"`
}

func CleanString(in string) string {
	out := strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, in)

	return out
}

func GetAuthToken(creds RegistryCreds) string {
	jsonBody, err := json.Marshal(creds)
	if err != nil {
		panic(err)
	}
	sEnc := b64.StdEncoding.EncodeToString(jsonBody)

	return sEnc
}
