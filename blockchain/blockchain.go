package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/dgraph-io/badger"
)

const (
	dbPath      = "./tmp/blocks"
	dbFile      = "./tmp/blocks/MANIFEST"
	genesisData = "First Transaction from Genesis"
)

type BlockChain struct {
	LastHash []byte
	Database *badger.DB
}

type BlockChainIterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

// Initialization the chain for the address
func InitBlockChain(address string) *BlockChain {
	if DBexists() {
		fmt.Println("Blockchain already exists!!!")
		runtime.Goexit()
	}

	var lastHash []byte

	// configure the database
	opts := badger.DefaultOptions(dbPath)

	// open the database
	db, err := badger.Open(opts)
	ErrorHandler(err)

	err = db.Update(func(txn *badger.Txn) error {
		cbtx := CoinBaseTx(address, genesisData)
		genesis := Genesis(cbtx)
		fmt.Println("Genesis created")

		err = txn.Set(genesis.Hash, genesis.Serialize())
		ErrorHandler(err)

		err = txn.Set([]byte("lh"), genesis.Hash)

		lastHash = genesis.Hash

		return err
	})

	ErrorHandler(err)

	blockchain := BlockChain{lastHash, db}

	return &blockchain
}

// Get the  current chain for the address
func CountinueBlockChain(address string) *BlockChain {
	if !DBexists() {
		fmt.Println("No existing Blockchain found, create one !!!")
		runtime.Goexit()
	}

	var lastHash []byte

	// configure the database
	opts := badger.DefaultOptions(dbPath)

	// open the database
	db, err := badger.Open(opts)
	ErrorHandler(err)

	err = db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		ErrorHandler(err)

		err = item.Value(func(val []byte) error {
			lastHash = val
			return nil
		})

		return err
	})
	ErrorHandler(err)

	chain := BlockChain{lastHash, db}

	return &chain
}

func (chain *BlockChain) AddBlock(txs []*Transaction) *Block {
	var lastHash []byte

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		ErrorHandler(err)

		err = item.Value(func(val []byte) error {
			lastHash = val
			return nil
		})

		return err
	})

	ErrorHandler(err)

	// create a new block with the last hash
	newBlock := CreateBlock(txs, lastHash)

	err = chain.Database.Update(func(txn *badger.Txn) error {
		err := txn.Set(newBlock.Hash, newBlock.Serialize())
		ErrorHandler(err)
		err = txn.Set([]byte("lh"), newBlock.Hash)

		chain.LastHash = newBlock.Hash

		return err
	})

	ErrorHandler(err)

	return newBlock
}

// Convert the blockchain into iterator
func (chain *BlockChain) Iterator() *BlockChainIterator {
	iter := &BlockChainIterator{chain.LastHash, chain.Database}
	return iter
}

func (iter *BlockChainIterator) Next() *Block {
	var block *Block

	err := iter.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get(iter.CurrentHash)
		ErrorHandler(err)

		err = item.Value(func(encodedBlock []byte) error {
			block = Deserialize(encodedBlock)
			return nil
		})

		return err
	})

	ErrorHandler(err)

	iter.CurrentHash = block.PrevHash

	return block
}

func DBexists() bool {
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		return false
	}
	return true
}

// Retreive all the output transactions
func (chain *BlockChain) FindUTXO() map[string]TxOutputs {
	UTXO := make(map[string]TxOutputs)

	spentTXOs := make(map[string][]int)

	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Outputs {
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] {
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}
				outs := UTXO[txID]
				outs.Outputs = append(outs.Outputs, out)
				UTXO[txID] = outs
			}
			if !tx.IsCoinbase() {
				for _, in := range tx.Inputs {
					inTxID := hex.EncodeToString(in.ID)
					spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Out)
				}
			}
		}

		if len(block.PrevHash) == 0 {
			break
		}
	}
	return UTXO
}

// Search a transaction into the chain by the ID
func (chain *BlockChain) FindTransaction(ID []byte) (Transaction, error) {
	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			if bytes.Compare(tx.ID, ID) == 0 {
				return *tx, nil
			}
		}

		// breaj=k the loop at the last block of the chain
		if len(block.PrevHash) == 0 {
			break
		}
	}

	return Transaction{}, errors.New("Transaction doesn't exist!!!")
}

// Function to sign a transaction into the chain
func (chain *BlockChain) SignTransaction(tx *Transaction, privKey ecdsa.PrivateKey) {
	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs {
		prevTX, err := chain.FindTransaction(in.ID)
		ErrorHandler(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	tx.Sign(privKey, prevTXs)
}

// Function to verify a transaction into the chain
func (chain *BlockChain) VerifyTransaction(tx *Transaction) bool {
	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs {
		prevTX, err := chain.FindTransaction(in.ID)
		ErrorHandler(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	return tx.Verify(prevTXs)
}
