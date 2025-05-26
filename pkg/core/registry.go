package core

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const notebookPrefix = "notebook:"

type BadgerRegistry struct {
	db   *badger.DB
	subs []func(Notebook, RegistryAction)
}

func NewBadgerRegistry(dbPath string, subscribers ...func(Notebook, RegistryAction)) (*BadgerRegistry, error) {
	opts := badger.DefaultOptions(dbPath)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger db: %w", err)
	}

	reg := &BadgerRegistry{
		db:   db,
		subs: subscribers,
	}

	err = reg.loadExistingNotebooks()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to load existing notebooks: %w", err)
	}

	return reg, nil
}

func (r *BadgerRegistry) Close() error {
	return r.db.Close()
}

func (r *BadgerRegistry) Add(req CreateUpdateNotebookRequest) (Notebook, error) {
	log.Debug().Str("method", "BadgerRegistry.Add").
		Interface("request", req).Msg("Starting Add operation")

	if req.Name == "" || req.Path == "" || req.Domain == "" {
		return Notebook{}, fmt.Errorf("name, path, and domain are required for creation")
	}

	if _, exists := r.GetByDomain(req.Domain); exists {
		return Notebook{}, fmt.Errorf("domain %s is already in use", req.Domain)
	}

	nb := Notebook{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Path:      req.Path,
		Domain:    req.Domain,
		ShowCode:  req.ShowCode != nil && *req.ShowCode,
		Watch:     req.Watch != nil && *req.Watch,
		CreatedAt: time.Now(),
	}

	if _, exists := r.getNotebookByDomain(req.Domain); exists {
		return Notebook{}, fmt.Errorf("domain %s is already in use", req.Domain)
	}

	if err := r.storeNotebook(nb); err != nil {
		log.Error().Err(err).Str("id", nb.ID).Msg("Failed to store notebook")
		return Notebook{}, err
	}

	log.Debug().Str("id", nb.ID).Msg("Notifying subscribers")
	r.notifySubscribers(nb, ActionAdd)

	log.Info().Str("id", nb.ID).Str("domain", nb.Domain).
		Str("method", "BadgerRegistry.Add").
		Msg("Successfully added notebook")
	return nb, nil
}

func (r *BadgerRegistry) Get(id string) (Notebook, bool) {
	return r.getNotebook(id)
}

func (r *BadgerRegistry) GetByDomain(domain string) (Notebook, bool) {
	log.Debug().Str("method", "BadgerRegistry.GetByDomain").
		Str("domain", domain).Msg("Starting GetByDomain operation")
	var result Notebook
	var found bool

	err := r.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(notebookPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			var nb Notebook
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &nb)
			}); err != nil {
				log.Warn().Err(err).
					Str("method", "BadgerRegistry.GetByDomain").
					Msg("Failed to unmarshal notebook")
				continue
			}

			if nb.Domain == domain {
				result = nb
				found = true
				return nil
			}
		}
		log.Debug().Str("method", "BadgerRegistry.GetByDomain").
			Str("domain", domain).Msg("No notebook found")
		return nil
	})

	if err != nil {
		log.Error().Err(err).Str("method", "BadgerRegistry.GetByDomain").
			Str("domain", domain).Msg("Failed to get notebook by domain")
		return Notebook{}, false
	}
	return result, found
}

func (r *BadgerRegistry) List() []Notebook {
	var notebooks []Notebook
	_ = r.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(notebookPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			var nb Notebook
			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &nb)
			})
			if err == nil {
				notebooks = append(notebooks, nb)
			} else {
				log.Warn().Err(err).Str("method", "BadgerRegistry.List").Msg("Failed to unmarshal notebook")
			}
		}
		return nil
	})
	log.Debug().Str("method", "BadgerRegistry.List").Int("count", len(notebooks)).Msg("Successfully listed notebooks")
	return notebooks
}

func (r *BadgerRegistry) Update(id string, req CreateUpdateNotebookRequest) (Notebook, error) {
	log.Debug().Str("method", "BadgerRegistry.Update").
		Str("id", id).
		Interface("req", req).
		Msg("Starting Update operation")

	if req.Domain != "" {
		if existing, exists := r.GetByDomain(req.Domain); exists && existing.ID != id {
			return Notebook{}, fmt.Errorf("domain %s is already in use", req.Domain)
		}
	}

	nb, exists := r.getNotebook(id)

	if !exists {
		log.Warn().Str("id", id).Msg("Notebook not found")
		return Notebook{}, fmt.Errorf("notebook %s not found", id)
	}

	updated := false
	if req.Name != "" && req.Name != nb.Name {
		nb.Name = req.Name
		updated = true
	}
	if req.Path != "" && req.Path != nb.Path {
		nb.Path = req.Path
		updated = true
	}
	if req.Domain != "" && req.Domain != nb.Domain {
		nb.Domain = req.Domain
		updated = true
	}
	if req.ShowCode != nil && *req.ShowCode != nb.ShowCode {
		nb.ShowCode = *req.ShowCode
		updated = true
	}
	if req.Watch != nil && *req.Watch != nb.Watch {
		nb.Watch = *req.Watch
		updated = true
	}

	if !updated {
		log.Debug().Str("method", "BadgerRegistry.Update").
			Str("id", id).
			Msg("No changes to update")
		return nb, nil
	}

	if req.Domain != "" && req.Domain != nb.Domain {
		if existing, exists := r.getNotebookByDomain(req.Domain); exists && existing.ID != id {
			return Notebook{}, fmt.Errorf("domain %s is already in use", req.Domain)
		}
	}

	if err := r.storeNotebook(nb); err != nil {
		return Notebook{}, err
	}

	log.Debug().Str("method", "BadgerRegistry.Update").Str("id", id).Msg("Notifying subscribers")
	r.notifySubscribers(nb, ActionUpdate)

	log.Info().Str("id", id).Str("domain", nb.Domain).Msg("Successfully updated notebook")
	return nb, nil
}

func (r *BadgerRegistry) Delete(id string) error {
	log.Debug().Str("method", "BadgerRegistry.Delete").Str("id", id).Msg("Starting Delete operation")

	nb, exists := r.getNotebook(id)

	if !exists {
		return fmt.Errorf("notebook %s not found", id)
	}

	if _, exists := r.getNotebook(id); !exists {
		return fmt.Errorf("notebook %s was deleted concurrently", id)
	}

	log.Debug().Str("id", id).Msg("Deleting notebook from storage")
	err := r.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(notebookPrefix + id))
	})
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to delete notebook")
		return err
	}

	log.Debug().Str("method", "BadgerRegistry.Delete").Str("id", id).Msg("Notifying subscribers")
	r.notifySubscribers(nb, ActionDelete)

	log.Info().Str("id", id).Str("domain", nb.Domain).Msg("Successfully deleted notebook")
	return nil
}

func (r *BadgerRegistry) loadExistingNotebooks() error {
	return r.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(notebookPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			var nb Notebook
			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &nb)
			})
			if err != nil {
				log.Warn().Err(err).
					Str("method", "BadgerRegistry.loadExistingNotebooks").
					Msg("Failed to unmarshal notebook")
				continue
			}
			log.Debug().Str("id", nb.ID).Msg("Notifying subscribers for loaded notebook")
			r.notifySubscribers(nb, ActionAdd)
		}
		return nil
	})
}

func (r *BadgerRegistry) getNotebook(id string) (Notebook, bool) {
	var nb Notebook
	err := r.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(notebookPrefix + id))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &nb)
		})
	})

	if err != nil {
		log.Warn().Err(err).
			Str("method", "BadgerRegistry.getNotebook").
			Str("id", id).
			Msg("Failed to get notebook")
		return Notebook{}, false
	}
	return nb, true
}

func (r *BadgerRegistry) storeNotebook(nb Notebook) error {
	log.Debug().Str("method", "BadgerRegistry.storeNotebook").
		Interface("notebook", nb).Msg("Storing notebook")
	data, err := json.Marshal(nb)
	if err != nil {
		log.Warn().Err(err).
			Str("method", "BadgerRegistry.storeNotebook").
			Str("id", nb.ID).
			Msg("Failed to marshal notebook")
		return err
	}

	return r.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(notebookPrefix+nb.ID), data)
	})
}

func (r *BadgerRegistry) notifySubscribers(nb Notebook, action RegistryAction) {
	log.Debug().Str("method", "BadgerRegistry.notifySubscribers").
		Interface("notebook", nb).
		Interface("action", action).
		Int("subscribers", len(r.subs)).
		Msg("Notifying subscribers")
	for _, handler := range r.subs {
		go handler(nb, action)
	}
}

func (r *BadgerRegistry) getNotebookByDomain(domain string) (Notebook, bool) {
	log.Debug().Str("method", "BadgerRegistry.getNotebookByDomain").
		Str("domain", domain).
		Msg("Starting getNotebookByDomain operation")
	var result Notebook
	var found bool

	err := r.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(notebookPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			var nb Notebook
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &nb)
			}); err != nil {
				log.Warn().Err(err).
					Str("method", "BadgerRegistry.getNotebookByDomain").
					Msg("Failed to unmarshal notebook")
				continue
			}

			if nb.Domain == domain {
				result = nb
				found = true
				return nil
			}
		}
		log.Debug().Str("method", "BadgerRegistry.getNotebookByDomain").
			Str("domain", domain).
			Msg("No notebook found")
		return nil
	})

	if err != nil {
		return Notebook{}, false
	}
	return result, found
}
