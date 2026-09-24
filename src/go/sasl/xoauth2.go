package sasl

import (
	"fmt"
	"net/smtp"
)

// XOAuth2 implements the XOAUTH2 SASL mechanism for Microsoft 365.
// Compatible with both net/smtp and github.com/emersion/go-sasl.
type XOAuth2 struct {
	Username    string
	AccessToken string
}

// NewXOAuth2 creates a new XOAuth2 SASL client.
func NewXOAuth2(username, accessToken string) *XOAuth2 {
	return &XOAuth2{
		Username:    username,
		AccessToken: accessToken,
	}
}

// Start implements smtp.Auth for SMTP usage.
func (x *XOAuth2) Start(*smtp.ServerInfo) (string, []byte, error) {
	resp := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", x.Username, x.AccessToken)
	return "XOAUTH2", []byte(resp), nil
}

// Next implements smtp.Auth. Microsoft may return a JSON error challenge.
func (x *XOAuth2) Next(_ []byte, _ bool) ([]byte, error) {
	return []byte{}, nil
}

// Client implements sasl.Client from github.com/emersion/go-sasl.
type Client struct {
	Username    string
	AccessToken string
}

// NewClient creates a new XOAuth2 SASL client for go-sasl.
func NewClient(username, accessToken string) *Client {
	return &Client{
		Username:    username,
		AccessToken: accessToken,
	}
}

// Start implements sasl.Client.
func (x *Client) Start() (string, []byte, error) {
	resp := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", x.Username, x.AccessToken)
	return "XOAUTH2", []byte(resp), nil
}

// Next implements sasl.Client.
func (x *Client) Next(_ []byte) ([]byte, error) {
	return []byte{}, nil
}
