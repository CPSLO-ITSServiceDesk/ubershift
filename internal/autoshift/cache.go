package autoshift

import (
	"context"
	"log"
	"os"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"
)

type TokenCache struct {
	path string
}

func (t *TokenCache) Replace(
	ctx context.Context,
	c cache.Unmarshaler,
	hints cache.ReplaceHints,
) error {
	log.Println("Loading Microsoft token cache")
	data, err := os.ReadFile(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return c.Unmarshal(data)
}

func (t *TokenCache) Export(
	ctx context.Context,
	c cache.Marshaler,
	hints cache.ExportHints,
) error {
	data, err := c.Marshal()
	if err != nil {
		return err
	}
	return os.WriteFile(t.path, data, 0600)
}
