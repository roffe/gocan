package t8

import "context"

const (
	AccessLevel01 int = iota
	AccessLevelFB
	AccessLevelFD
)

// 0
func (t *Client) RequestSecurityAccess(ctx context.Context, accesslevel int) bool {

	return false
}
