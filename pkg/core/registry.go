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
	log.Debug().Str("method", "Add").Str("domain", req.Domain).Msg("Starting Add operation")

	if req.Name == "" || req.Path == "" || req.Domain == "" {
		log.Warn().Interface("request", req).Msg("Missing required fields")
		return Notebook{}, fmt.Errorf("name, path, and domain are required for creation")
	}

	log.Debug().Str("domain", req.Domain).Msg("Checking if domain exists")
	if _, exists := r.GetByDomain(req.Domain); exists {
		log.Warn().Str("domain", req.Domain).Msg("Domain already in use")
		return Notebook{}, fmt.Errorf("domain %s is already in use", req.Domain)
	}

	log.Debug().Msg("Creating new notebook")
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

	log.Debug().Str("id", nb.ID).Msg("Storing notebook")
	if err := r.storeNotebook(nb); err != nil {
		log.Error().Err(err).Str("id", nb.ID).Msg("Failed to store notebook")
		return Notebook{}, err
	}

	log.Debug().Str("id", nb.ID).Msg("Notifying subscribers")
	r.notifySubscribers(nb, ActionAdd)

	log.Info().Str("id", nb.ID).Str("domain", nb.Domain).Msg("Successfully added notebook")
	return nb, nil
}

func (r *BadgerRegistry) Get(id string) (Notebook, bool) {
	return r.getNotebook(id)
}

func (r *BadgerRegistry) GetByDomain(domain string) (Notebook, bool) {
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
				continue
			}

			if nb.Domain == domain {
				result = nb
				found = true
				return nil
			}
		}
		return nil
	})

	if err != nil {
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
			}
			// Optionally log error if item unmarshalling fails
		}
		return nil
	})
	return notebooks
}

func (r *BadgerRegistry) Update(id string, req CreateUpdateNotebookRequest) (Notebook, error) {
	log.Debug().Str("method", "Update").Str("id", id).Msg("Starting Update operation")

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
		log.Debug().Str("id", id).Msg("No changes to update")
		return nb, nil
	}

	log.Debug().Msg("Acquiring write lock for final domain check and storage")

	if req.Domain != "" && req.Domain != nb.Domain {
		if existing, exists := r.getNotebookByDomain(req.Domain); exists && existing.ID != id {
			log.Warn().Str("domain", req.Domain).Msg("Domain became in use after lock acquisition")
			return Notebook{}, fmt.Errorf("domain %s is already in use", req.Domain)
		}
	}

	log.Debug().Str("id", id).Msg("Storing updated notebook")
	if err := r.storeNotebook(nb); err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to store updated notebook")
		return Notebook{}, err
	}

	log.Debug().Str("id", id).Msg("Notifying subscribers")
	r.notifySubscribers(nb, ActionUpdate)

	log.Info().Str("id", id).Str("domain", nb.Domain).Msg("Successfully updated notebook")
	return nb, nil
}

func (r *BadgerRegistry) Delete(id string) error {
	log.Debug().Str("method", "Delete").Str("id", id).Msg("Starting Delete operation")

	nb, exists := r.getNotebook(id)

	if !exists {
		log.Warn().Str("id", id).Msg("Notebook not found")
		return fmt.Errorf("notebook %s not found", id)
	}

	log.Debug().Msg("Acquiring write lock for deletion")

	if _, exists := r.getNotebook(id); !exists {
		log.Warn().Str("id", id).Msg("Notebook was deleted concurrently")
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

	log.Debug().Str("id", id).Msg("Notifying subscribers")
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
				// Optionally log the error and skip to the next notebook
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
		return Notebook{}, false
	}
	return nb, true
}

func (r *BadgerRegistry) storeNotebook(nb Notebook) error {
	data, err := json.Marshal(nb)
	if err != nil {
		return err
	}

	return r.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(notebookPrefix+nb.ID), data)
	})
}

func (r *BadgerRegistry) notifySubscribers(nb Notebook, action RegistryAction) {
	for _, handler := range r.subs {
		// Run handlers in goroutines to prevent blocking the registry
		go handler(nb, action)
	}
}

func (r *BadgerRegistry) getNotebookByDomain(domain string) (Notebook, bool) {
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
				continue
			}

			if nb.Domain == domain {
				result = nb
				found = true
				return nil
			}
		}
		return nil
	})

	if err != nil {
		return Notebook{}, false
	}
	return result, found
}
