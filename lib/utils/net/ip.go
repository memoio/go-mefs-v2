package net

import (
	"bytes"
	"io/ioutil"
	"net/http"
)

func GetPulicIP() (string, error) {
	resp, err := http.Get("http://myexternalip.com/raw")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(content)), nil
}
