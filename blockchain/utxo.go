package blockchain

import (
	"bytes"
	"encoding/hex"

	"github.com/dgraph-io/badger"
)

var (
	utxoPrefix   = []byte("utxo-")
	prefixLength = len(utxoPrefix)
)

type UTXOSet struct {
	Blockchain *BlockChain
}

func (u UTXOSet) Reindex() {
	// open the DB instance from the chain
	db := u.Blockchain.Database

	u.DeleteByPrefix(utxoPrefix)

	UTXO := u.Blockchain.FindUTXO()

	// call a read/write transaction into DB
	err := db.Update(func(txn *badger.Txn) error {
		for txId, outs := range UTXO {
			key, err := hex.DecodeString(txId)
			if err != nil {
				return err
			}

			// configure the key with the default prefix
			key = append(utxoPrefix, key...)

			err = txn.Set(key, outs.Serialize())
			ErrorHandler(err)
		}
		return nil
	})
	ErrorHandler(err)
}

func (u *UTXOSet) Update(block *Block) {
	// open the DB instance from the chain
	db := u.Blockchain.Database

	// call a read/write transaction into DB
	err := db.Update(func(txn *badger.Txn) error {

		for _, tx := range block.Transactions {
			if !tx.IsCoinbase() {
				for _, in := range tx.Inputs {
					updatedOuts := TxOutputs{}
					inID := append(utxoPrefix, in.ID...)

					item, err := txn.Get(inID)
					ErrorHandler(err)

					var dst []byte
					v, err := item.ValueCopy(dst)
					ErrorHandler(err)
					outs := DeserializeOutputs(v)

					for outIdx, out := range outs.Outputs {
						if outIdx != in.Out {
							updatedOuts.Outputs = append(updatedOuts.Outputs, out)
						}
					}

					if len(updatedOuts.Outputs) == 0 {
						if err := txn.Delete(inID); err != nil {
							ErrorHandler(err)
						}
					} else {
						if err := txn.Set(inID, updatedOuts.Serialize()); err != nil {
							ErrorHandler(err)
						}
					}
				}
			}

			// create a new outputs structure
			newOutputs := TxOutputs{}
			for _, out := range tx.Outputs {
				newOutputs.Outputs = append(newOutputs.Outputs, out)
			}

			txID := append(utxoPrefix, tx.ID...)
			if err := txn.Set(txID, newOutputs.Serialize()); err != nil {
				ErrorHandler(err)
			}
		}

		return nil
	})
	ErrorHandler(err)
}

// Retreive all transactions without outputs
func (u UTXOSet) FindUnspentTransactions(pubKeyHash []byte) []TxOutput {
	var UTXOs []TxOutput

	// get DB instance of the chain
	db := u.Blockchain.Database

	// open a readOnly transaction into DB
	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(utxoPrefix); it.ValidForPrefix(utxoPrefix); it.Next() {
			item := it.Item()

			var dst []byte
			v, err := item.ValueCopy(dst)
			ErrorHandler(err)

			outs := DeserializeOutputs(v)

			for _, out := range outs.Outputs {
				if out.IsLockedWithKey(pubKeyHash) {
					UTXOs = append(UTXOs, out)
				}
			}
		}

		return nil
	})

	ErrorHandler(err)

	return UTXOs
}

// Retreive all the available output transaction for an amount
func (u UTXOSet) FindSpendabaleOutputs(pubKeyHash []byte, amount int) (int, map[string][]int) {
	unspentOuts := make(map[string][]int)
	accumulated := 0

	// get DB instance of the chain
	db := u.Blockchain.Database

	// open a readOnly transaction into DB
	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(utxoPrefix); it.ValidForPrefix(utxoPrefix); it.Next() {
			item := it.Item()
			k := item.Key()
			var dst []byte
			v, err := item.ValueCopy(dst)
			ErrorHandler(err)
			k = bytes.TrimPrefix(k, utxoPrefix)
			txID := hex.EncodeToString(k)
			outs := DeserializeOutputs(v)

			for outIdx, out := range outs.Outputs {
				// check if the address is good and add enough coins for the amount
				if out.IsLockedWithKey(pubKeyHash) && accumulated < amount {
					accumulated += out.Value
					unspentOuts[txID] = append(unspentOuts[txID], outIdx)
				}
			}
		}

		return nil
	})

	ErrorHandler(err)

	return accumulated, unspentOuts
}

// make a counter how many transaction unspent into the chain
func (u UTXOSet) CountTransactions() int {
	// get DB instance of the chain
	db := u.Blockchain.Database

	counter := 0

	// open a readOnly transaction into DB
	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(utxoPrefix); it.ValidForPrefix(utxoPrefix); it.Next() {
			counter++
		}

		return nil
	})

	ErrorHandler(err)

	return counter
}

func (u *UTXOSet) DeleteByPrefix(prefix []byte) {
	// open the read/write transaction to get all the keys need to be delete
	deleteKeys := func(keysForDelete [][]byte) error {
		if err := u.Blockchain.Database.Update(func(txn *badger.Txn) error {
			for _, key := range keysForDelete {
				if err := txn.Delete(key); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
		return nil
	}

	collectSize := 100000
	// iterate into DB with readOnly transaction and loop every 100000 keys
	u.Blockchain.Database.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		keysForDelete := make([][]byte, 0, collectSize)
		keyCollected := 0

		// search all the key starting by the prefix
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().KeyCopy(nil)
			keysForDelete = append(keysForDelete, key)
			keyCollected++
			if keyCollected == collectSize {
				if err := deleteKeys(keysForDelete); err != nil {
					ErrorHandler(err)
				}
				keysForDelete = make([][]byte, 0, collectSize)
				keyCollected = 0
			}
		}

		if keyCollected > 0 {
			if err := deleteKeys(keysForDelete); err != nil {
				ErrorHandler(err)
			}
		}

		return nil
	})
}
