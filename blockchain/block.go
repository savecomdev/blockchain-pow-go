package blockchain

import (
	"bytes"
	"encoding/gob"
	"log"
	"time"
)

type Block struct {
	Timestamp    int64
	Hash         []byte
	Transactions []*Transaction
	PrevHash     []byte
	Nonce        int
	Height       int
}

func ErrorHandler(err error) {
	if err != nil {
		log.Panic(err)
	}
}

func CreateBlock(txs []*Transaction, prevHash []byte, height int) *Block {
	block := &Block{time.Now().Unix(), []byte{}, txs, prevHash, 0, height}

	pow := NewProof(block)
	nonce, hash := pow.Run()

	block.Hash = hash[:]
	block.Nonce = nonce

	return block
}

func Genesis(coinbase *Transaction) *Block {
	return CreateBlock([]*Transaction{coinbase}, []byte{}, 0)
}

func (b *Block) Serialize() []byte {
	var res bytes.Buffer

	encoder := gob.NewEncoder(&res)

	err := encoder.Encode(b)

	ErrorHandler(err)

	return res.Bytes()
}

func Deserialize(data []byte) *Block {
	var block Block

	decoder := gob.NewDecoder(bytes.NewReader(data))

	err := decoder.Decode(&block)

	ErrorHandler(err)

	return &block
}

func (b *Block) HashTransaction() []byte {
	var txHashes [][]byte

	// populate the maps of transaction hashes
	for _, tx := range b.Transactions {
		txHashes = append(txHashes, tx.Serialize())
	}

	// create a tree nodes
	tree := NewMerkleTree(txHashes)
	return tree.RootNode.Data
}
