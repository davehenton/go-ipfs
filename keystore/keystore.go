package keystore

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	ci "gx/ipfs/QmP1DfoUjiWH2ZBo1PBH6FupdBucbDepx3HpWmEY6JMUpY/go-libp2p-crypto"
	peer "gx/ipfs/QmdS9KpbDyPrieswibZhkod1oXqRwZJrUPzxCofAMWpFGq/go-libp2p-peer"
)

type Keystore interface {
	// Has return whether or not a key exist in the Keystore
	Has(string) (bool, error)
	// Put store a key in the Keystore
	Put(string, ci.PrivKey) error
	// Get retrieve a key from the Keystore
	Get(string) (ci.PrivKey, error)
	// GetById retrieve gets private key assisted with the pubkeyhash
	GetById(peer.ID) (ci.PrivKey, error)
	// Delete remove a key from the Keystore
	Delete(string) error
	// List return a list of key identifier
	List() ([]string, error)
}

var ErrNoSuchKey = fmt.Errorf("no key by the given name was found")
var ErrKeyExists = fmt.Errorf("key by that name already exists, refusing to overwrite")

type FSKeystore struct {
	dir string
}

func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("key names must be at least one character")
	}

	if strings.Contains(name, "/") {
		return fmt.Errorf("key names may not contain slashes")
	}

	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("key names may not begin with a period")
	}

	return nil
}

func NewFSKeystore(dir string) (*FSKeystore, error) {
	_, err := os.Stat(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		if err := os.Mkdir(dir, 0700); err != nil {
			return nil, err
		}
	}

	return &FSKeystore{dir}, nil
}

// Has return whether or not a key exist in the Keystore
func (ks *FSKeystore) Has(name string) (bool, error) {
	kp := filepath.Join(ks.dir, name)

	_, err := os.Stat(kp)

	if os.IsNotExist(err) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, nil
}

// Put store a key in the Keystore
func (ks *FSKeystore) Put(name string, k ci.PrivKey) error {
	if err := validateName(name); err != nil {
		return err
	}

	b, err := k.Bytes()
	if err != nil {
		return err
	}

	kp := filepath.Join(ks.dir, name)

	_, err = os.Stat(kp)
	if err == nil {
		return ErrKeyExists
	} else if !os.IsNotExist(err) {
		return err
	}

	fi, err := os.Create(kp)
	if err != nil {
		return err
	}
	defer fi.Close()

	_, err = fi.Write(b)
	if err != nil {
		return err
	}

	return nil
}

// Get retrieve a key from the Keystore
func (ks *FSKeystore) Get(name string) (ci.PrivKey, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}

	kp := filepath.Join(ks.dir, name)

	data, err := ioutil.ReadFile(kp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoSuchKey
		}
		return nil, err
	}

	return ci.UnmarshalPrivateKey(data)
}

// GetById retrieve gets private key assisted with the pubkeyhash
func (ks *FSKeystore) GetById(want peer.ID) (ci.PrivKey, error) {
	names, err := ks.List()
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, name := range names {
		sk, err := ks.Get(name)
		if err != nil {
			// don't abort on error as we may still be able to find
			// the key
			lastErr = err
			continue
		}
		id, err := peer.IDFromPrivateKey(sk)
		if err != nil {
			lastErr = err
			continue
		}
		if want == id {
			return sk, nil
		}
	}
	if lastErr == nil {
		return nil, ErrNoSuchKey
	} else {
		return nil, lastErr
	}
}

// Delete remove a key from the Keystore
func (ks *FSKeystore) Delete(name string) error {
	if err := validateName(name); err != nil {
		return err
	}

	kp := filepath.Join(ks.dir, name)

	return os.Remove(kp)
}

// List return a list of key identifier
func (ks *FSKeystore) List() ([]string, error) {
	dir, err := os.Open(ks.dir)
	if err != nil {
		return nil, err
	}

	return dir.Readdirnames(0)
}
