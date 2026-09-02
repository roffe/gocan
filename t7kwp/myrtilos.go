package t7kwp

import (
	"context"
	"fmt"
)

func (t *Client) SendEU0DRegistrationKey(ctx context.Context, key []byte) error {
	if len(key) != 4 {
		return fmt.Errorf("SetEU0DRegistrationKey[1]: key must be 4 bytes")
	}
	payload := []byte{0x40, 0xA1, 0x05, EU0D_SET_REGISTRATION_KEY, key[0], key[1], key[2], key[3]}
	resp, err := t.request(ctx, REQ_MSG_ID, payload, DefaultTimeout, t.responseID)
	if err != nil {
		return fmt.Errorf("SetEU0DRegistrationKey[2]: %w", err)
	}
	return checkErr(resp)
}

func (t *Client) SendEU0DRegistrationKeyLong(ctx context.Context, key []byte) error {
	return t.sendLong(ctx, append([]byte{0x06, EU0D_SET_REGISTRATION_KEY}, key...))
}
