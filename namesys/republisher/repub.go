package republisher

import (
	"context"
	"errors"
	"sync"
	"time"

	keystore "github.com/ipfs/go-ipfs/keystore"
	namesys "github.com/ipfs/go-ipfs/namesys"
	pb "github.com/ipfs/go-ipfs/namesys/pb"
	path "github.com/ipfs/go-ipfs/path"
	dshelp "github.com/ipfs/go-ipfs/thirdparty/ds-help"

	ic "gx/ipfs/QmP1DfoUjiWH2ZBo1PBH6FupdBucbDepx3HpWmEY6JMUpY/go-libp2p-crypto"
	ds "gx/ipfs/QmRWDav6mzWseLWeYfVd5fvUKiVe9xNH29YfMF438fG364/go-datastore"
	goprocess "gx/ipfs/QmSF8fPo3jgVBAy8fpdjjYqgG87dkJgUprRBHRd2tmfgpP/goprocess"
	gpctx "gx/ipfs/QmSF8fPo3jgVBAy8fpdjjYqgG87dkJgUprRBHRd2tmfgpP/goprocess/context"
	logging "gx/ipfs/QmSpJByNKFX1sCsHBEp3R73FL4NF6FnQTEGyNAXHm2GS52/go-log"
	recpb "gx/ipfs/QmWYCqr6UDqqD1bfRybaAPtbAqcN3TSJpveaBXMwbQ3ePZ/go-libp2p-record/pb"
	routing "gx/ipfs/QmXiH3yLocPhjkAmL8R29fKRcEKoVXKCaVDbAS9tdTrVEd/go-libp2p-routing"
	proto "gx/ipfs/QmZ4Qi3GaRbjcx28Sme5eMH7RQjGkt8wHxt2a65oLaeFEV/gogo-protobuf/proto"
	peer "gx/ipfs/QmdS9KpbDyPrieswibZhkod1oXqRwZJrUPzxCofAMWpFGq/go-libp2p-peer"
)

var errNoEntry = errors.New("no previous entry")

var log = logging.Logger("ipns-repub")

var DefaultRebroadcastInterval = time.Hour * 4

const DefaultRecordLifetime = time.Hour * 24

type Republisher struct {
	r    routing.ValueStore
	ds   ds.Datastore
	self ic.PrivKey
	ks   keystore.Keystore

	Interval time.Duration

	// how long records that are republished should be valid for
	RecordLifetime time.Duration

	entrylock sync.Mutex
	entries   map[peer.ID]struct{}
}

func NewRepublisher(r routing.ValueStore, ds ds.Datastore, self ic.PrivKey, ks keystore.Keystore) *Republisher {
	return &Republisher{
		r:              r,
		ds:             ds,
		self:           self,
		ks:             ks,
		entries:        make(map[peer.ID]struct{}),
		Interval:       DefaultRebroadcastInterval,
		RecordLifetime: DefaultRecordLifetime,
	}
}

func (rp *Republisher) AddName(id peer.ID) {
	rp.entrylock.Lock()
	defer rp.entrylock.Unlock()
	rp.entries[id] = struct{}{}
}

func (rp *Republisher) Run(proc goprocess.Process) {
	tick := time.NewTicker(rp.Interval)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			err := rp.republishEntries(proc)
			if err != nil {
				log.Error("Republisher failed to republish: ", err)
			}
		case <-proc.Closing():
			return
		}
	}
}

func (rp *Republisher) republishEntries(p goprocess.Process) error {
	ctx, cancel := context.WithCancel(gpctx.OnClosingContext(p))
	defer cancel()

	for id, _ := range rp.entries {
		log.Debugf("republishing ipns entry for %s", id)
		var priv ic.PrivKey
		selfId, err := peer.IDFromPrivateKey(rp.self)
		if err != nil {
			return err
		}
		if id == selfId {
			priv = rp.self
		} else {
			priv, err = rp.ks.GetById(id)
			if err != nil {
				return err
			}
		}

		// Look for it locally only
		_, ipnskey := namesys.IpnsKeysForID(id)
		p, seq, err := rp.getLastVal(ipnskey)
		if err != nil {
			if err == errNoEntry {
				continue
			}
			return err
		}

		// update record with same sequence number
		eol := time.Now().Add(rp.RecordLifetime)
		err = namesys.PutRecordToRouting(ctx, priv, p, seq, eol, rp.r, id)
		if err != nil {
			println("put record to routing error: " + err.Error())
			return err
		}
	}

	return nil
}

func (rp *Republisher) getLastVal(k string) (path.Path, uint64, error) {
	ival, err := rp.ds.Get(dshelp.NewKeyFromBinary([]byte(k)))
	if err != nil {
		// not found means we dont have a previously published entry
		return "", 0, errNoEntry
	}

	val := ival.([]byte)
	dhtrec := new(recpb.Record)
	err = proto.Unmarshal(val, dhtrec)
	if err != nil {
		return "", 0, err
	}

	// extract published data from record
	e := new(pb.IpnsEntry)
	err = proto.Unmarshal(dhtrec.GetValue(), e)
	if err != nil {
		return "", 0, err
	}
	return path.Path(e.Value), e.GetSequence(), nil
}
