package cfg

import (
	"encoding/json"
	"os"
	"path"

	"github.com/adrg/xdg"
	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
)

type ConfigManager struct {
	path    string
	loaded  ConfigV1
	log     *log.Entry
	changed []chan ConfigChangedEvent
}

type ConfigChangedEvent struct{}

var manager *ConfigManager

func Manager() *ConfigManager {
	if manager == nil {
		m, err := newManager()
		if err != nil {
			panic(err)
		}
		manager = m
	}
	return manager
}

func newManager() (*ConfigManager, error) {
	file, err := xdg.ConfigFile("authentik/config.json")
	if err != nil {
		return nil, err
	}
	cfg := &ConfigManager{
		path:    file,
		log:     log.WithField("logger", "config"),
		changed: make([]chan ConfigChangedEvent, 0),
	}
	cfg.log.WithField("path", file).Debug("Config file path")
	err = cfg.Load()
	if err != nil {
		return nil, err
	}
	cfg.log.Debug("Starting config watch")
	err = cfg.watch()
	if err != nil {
		return nil, err
	}
	// Automatically watch and reload config
	go func() {
		for range cfg.Watch() {
			cfg.log.Debug("config file changed, triggering config reload")
			err = cfg.Load()
			if err != nil {
				cfg.log.WithError(err).Warning("failed to reload config")
				continue
			}
		}
	}()
	return cfg, nil
}

func (cfg *ConfigManager) Load() error {
	f, err := os.Open(cfg.path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg.log.WithError(err).Debug("no config found, defaulting to empty")
			cfg.loaded = ConfigV1Default()
			return nil
		}
		return err
	}
	defer f.Close()
	err = json.NewDecoder(f).Decode(&cfg.loaded)
	if err != nil {
		return err
	}
	return nil
}

func (cfg *ConfigManager) Get() ConfigV1 {
	return cfg.loaded
}

func (cfg *ConfigManager) Save() error {
	f, err := os.OpenFile(cfg.path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil && !os.IsExist(err) && !os.IsNotExist(err) {
		return err
	}
	defer f.Close()
	err = json.NewEncoder(f).Encode(&cfg.loaded)
	if err != nil {
		return err
	}
	return nil
}

func (cfg *ConfigManager) Watch() chan ConfigChangedEvent {
	ch := make(chan ConfigChangedEvent)
	cfg.changed = append(cfg.changed, make(chan ConfigChangedEvent))
	defer func() {
		// Trigger config changed just after this function is called
		ch <- ConfigChangedEvent{}
	}()
	return ch
}

func (cfg *ConfigManager) watch() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					continue
				}
				cfg.log.WithField("event", event).Debug("file watch event")
				if event.Name == cfg.path && event.Has(fsnotify.Write) {
					for _, ch := range cfg.changed {
						ch <- ConfigChangedEvent{}
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					continue
				}
				cfg.log.WithError(err).Warning("error watching file")
			}
		}
	}()

	err = watcher.Add(path.Dir(cfg.path))
	if err != nil {
		return err
	}
	return nil
}
