package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/adrg/xdg"
	log "github.com/sirupsen/logrus"
)

type CacheData interface {
	Expiry() time.Time
}

type Cache[T CacheData] struct {
	uid  string
	path string
	log  *log.Entry
}

func NewCache[T CacheData](uidParts ...string) *Cache[T] {
	uid := strings.ReplaceAll(strings.Join(uidParts, "-"), "/", "_")
	c := &Cache[T]{
		uid: uid,
		log: log.WithField("logger", "cache").WithField("uid", uid),
	}
	p, _ := xdg.ConfigFile(fmt.Sprintf("authentik/cache/%s.json", c.uid))
	c.path = p
	return c
}

func (c *Cache[T]) Set(val T) error {
	c.log.Debug("Writing to cache")
	f, err := os.OpenFile(c.path, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
	if err != nil && !os.IsExist(err) && !os.IsNotExist(err) {
		return err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			c.log.WithError(err).Warning("failed to close cache file")
		}
	}()
	err = json.NewEncoder(f).Encode(val)
	if err != nil {
		return err
	}
	return nil
}

func (c *Cache[T]) Get() (T, error) {
	var cc T
	c.log.Debug("Checking cache")
	f, err := os.Open(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			c.log.WithError(err).Debug("No cache found")
			return cc, err
		}
		return cc, err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			c.log.WithError(err).Warning("failed to close cache file")
		}
	}()
	err = json.NewDecoder(f).Decode(&cc)
	if err != nil {
		return cc, err
	}
	if cc.Expiry().Before(time.Now()) {
		return cc, errors.New("cache expired")
	}
	return cc, nil
}
