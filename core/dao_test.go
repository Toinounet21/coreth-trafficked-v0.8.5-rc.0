// (c) 2021-2022, Ava Labs, Inc.
//
// This file is a derived work, based on the go-ethereum library whose original
// notices appear below.
//
// It is distributed under a license compatible with the licensing terms of the
// original code from which it is derived.
//
// Much love to the original authors for their work.
// **********
// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"math/big"
	"testing"

	"github.com/Toinounet21/coreth-trafficked-v0.8.5-rc.0/consensus/dummy"
	"github.com/Toinounet21/coreth-trafficked-v0.8.5-rc.0/core/rawdb"
	"github.com/Toinounet21/coreth-trafficked-v0.8.5-rc.0/core/vm"
	"github.com/Toinounet21/coreth-trafficked-v0.8.5-rc.0/params"
	"github.com/ethereum/go-ethereum/common"
)

// Tests that DAO-fork enabled clients can properly filter out fork-commencing
// blocks based on their extradata fields.
func TestDAOForkRangeExtradata(t *testing.T) {
	forkBlock := big.NewInt(32)

	// Generate a common prefix for both pro-forkers and non-forkers
	db := rawdb.NewMemoryDatabase()
	gspec := &Genesis{
		BaseFee: big.NewInt(params.ApricotPhase3InitialBaseFee),
		Config:  params.TestApricotPhase2Config,
	}
	genesis := gspec.MustCommit(db)
	prefix, _, _ := GenerateChain(params.TestApricotPhase2Config, genesis, dummy.NewFaker(), db, int(forkBlock.Int64()-1), 10, func(i int, gen *BlockGen) {})

	// Create the concurrent, conflicting two nodes
	proDb := rawdb.NewMemoryDatabase()
	gspec.MustCommit(proDb)

	proConf := *params.TestApricotPhase2Config
	proConf.DAOForkBlock = forkBlock
	proConf.DAOForkSupport = true

	proBc, _ := NewBlockChain(proDb, DefaultCacheConfig, &proConf, dummy.NewFaker(), vm.Config{}, common.Hash{})
	defer proBc.Stop()

	conDb := rawdb.NewMemoryDatabase()
	gspec.MustCommit(conDb)

	conConf := *params.TestApricotPhase2Config
	conConf.DAOForkBlock = forkBlock
	conConf.DAOForkSupport = false

	conBc, _ := NewBlockChain(conDb, DefaultCacheConfig, &conConf, dummy.NewFaker(), vm.Config{}, common.Hash{})
	defer conBc.Stop()

	if _, err := proBc.InsertChain(prefix); err != nil {
		t.Fatalf("pro-fork: failed to import chain prefix: %v", err)
	}
	if _, err := conBc.InsertChain(prefix); err != nil {
		t.Fatalf("con-fork: failed to import chain prefix: %v", err)
	}
	// Try to expand both pro-fork and non-fork chains iteratively with other camp's blocks
	for i := int64(0); i < params.DAOForkExtraRange.Int64(); i++ {
		// Create a pro-fork block, and try to feed into the no-fork chain
		db = rawdb.NewMemoryDatabase()
		gspec.MustCommit(db)
		bc, _ := NewBlockChain(db, DefaultCacheConfig, &conConf, dummy.NewFaker(), vm.Config{}, common.Hash{})
		defer bc.Stop()

		blocks := conBc.GetBlocksFromHash(conBc.CurrentBlock().Hash(), int(conBc.CurrentBlock().NumberU64()))
		for j := 0; j < len(blocks)/2; j++ {
			blocks[j], blocks[len(blocks)-1-j] = blocks[len(blocks)-1-j], blocks[j]
		}
		if _, err := bc.InsertChain(blocks); err != nil {
			t.Fatalf("failed to import contra-fork chain for expansion: %v", err)
		}
		if err := bc.stateCache.TrieDB().Commit(bc.CurrentHeader().Root, true, nil); err != nil {
			t.Fatalf("failed to commit contra-fork head for expansion: %v", err)
		}
		blocks, _, _ = GenerateChain(&proConf, conBc.CurrentBlock(), dummy.NewFaker(), db, 1, 10, func(i int, gen *BlockGen) {})
		if _, err := conBc.InsertChain(blocks); err != nil {
			t.Fatalf("contra-fork chain accepted pro-fork block: %v", blocks[0])
		}
		// Create a proper no-fork block for the contra-forker
		blocks, _, _ = GenerateChain(&conConf, conBc.CurrentBlock(), dummy.NewFaker(), db, 1, 10, func(i int, gen *BlockGen) {})
		if _, err := conBc.InsertChain(blocks); err != nil {
			t.Fatalf("contra-fork chain didn't accepted no-fork block: %v", err)
		}
		// Create a no-fork block, and try to feed into the pro-fork chain
		db = rawdb.NewMemoryDatabase()
		gspec.MustCommit(db)
		bc, _ = NewBlockChain(db, DefaultCacheConfig, &proConf, dummy.NewFaker(), vm.Config{}, common.Hash{})
		defer bc.Stop()

		blocks = proBc.GetBlocksFromHash(proBc.CurrentBlock().Hash(), int(proBc.CurrentBlock().NumberU64()))
		for j := 0; j < len(blocks)/2; j++ {
			blocks[j], blocks[len(blocks)-1-j] = blocks[len(blocks)-1-j], blocks[j]
		}
		if _, err := bc.InsertChain(blocks); err != nil {
			t.Fatalf("failed to import pro-fork chain for expansion: %v", err)
		}
		if err := bc.stateCache.TrieDB().Commit(bc.CurrentHeader().Root, true, nil); err != nil {
			t.Fatalf("failed to commit pro-fork head for expansion: %v", err)
		}
		blocks, _, _ = GenerateChain(&conConf, proBc.CurrentBlock(), dummy.NewFaker(), db, 1, 10, func(i int, gen *BlockGen) {})
		if _, err := proBc.InsertChain(blocks); err != nil {
			t.Fatalf("pro-fork chain accepted contra-fork block: %v", blocks[0])
		}
		// Create a proper pro-fork block for the pro-forker
		blocks, _, _ = GenerateChain(&proConf, proBc.CurrentBlock(), dummy.NewFaker(), db, 1, 10, func(i int, gen *BlockGen) {})
		if _, err := proBc.InsertChain(blocks); err != nil {
			t.Fatalf("pro-fork chain didn't accepted pro-fork block: %v", err)
		}
	}
	// Verify that contra-forkers accept pro-fork extra-datas after forking finishes
	db = rawdb.NewMemoryDatabase()
	gspec.MustCommit(db)
	bc, _ := NewBlockChain(db, DefaultCacheConfig, &conConf, dummy.NewFaker(), vm.Config{}, common.Hash{})
	defer bc.Stop()

	blocks := conBc.GetBlocksFromHash(conBc.CurrentBlock().Hash(), int(conBc.CurrentBlock().NumberU64()))
	for j := 0; j < len(blocks)/2; j++ {
		blocks[j], blocks[len(blocks)-1-j] = blocks[len(blocks)-1-j], blocks[j]
	}
	if _, err := bc.InsertChain(blocks); err != nil {
		t.Fatalf("failed to import contra-fork chain for expansion: %v", err)
	}
	if err := bc.stateCache.TrieDB().Commit(bc.CurrentHeader().Root, true, nil); err != nil {
		t.Fatalf("failed to commit contra-fork head for expansion: %v", err)
	}
	blocks, _, _ = GenerateChain(&proConf, conBc.CurrentBlock(), dummy.NewFaker(), db, 1, 10, func(i int, gen *BlockGen) {})
	if _, err := conBc.InsertChain(blocks); err != nil {
		t.Fatalf("contra-fork chain didn't accept pro-fork block post-fork: %v", err)
	}
	// Verify that pro-forkers accept contra-fork extra-datas after forking finishes
	db = rawdb.NewMemoryDatabase()
	gspec.MustCommit(db)
	bc, _ = NewBlockChain(db, DefaultCacheConfig, &proConf, dummy.NewFaker(), vm.Config{}, common.Hash{})
	defer bc.Stop()

	blocks = proBc.GetBlocksFromHash(proBc.CurrentBlock().Hash(), int(proBc.CurrentBlock().NumberU64()))
	for j := 0; j < len(blocks)/2; j++ {
		blocks[j], blocks[len(blocks)-1-j] = blocks[len(blocks)-1-j], blocks[j]
	}
	if _, err := bc.InsertChain(blocks); err != nil {
		t.Fatalf("failed to import pro-fork chain for expansion: %v", err)
	}
	if err := bc.stateCache.TrieDB().Commit(bc.CurrentHeader().Root, true, nil); err != nil {
		t.Fatalf("failed to commit pro-fork head for expansion: %v", err)
	}
	blocks, _, _ = GenerateChain(&conConf, proBc.CurrentBlock(), dummy.NewFaker(), db, 1, 10, func(i int, gen *BlockGen) {})
	if _, err := proBc.InsertChain(blocks); err != nil {
		t.Fatalf("pro-fork chain didn't accept contra-fork block post-fork: %v", err)
	}
}

func TestDAOForkSupportPostApricotPhase3(t *testing.T) {
	forkBlock := big.NewInt(0)

	conf := *params.TestChainConfig
	conf.DAOForkSupport = true
	conf.DAOForkBlock = forkBlock

	db := rawdb.NewMemoryDatabase()
	gspec := &Genesis{
		BaseFee: big.NewInt(params.ApricotPhase3InitialBaseFee),
		Config:  &conf,
	}
	genesis := gspec.MustCommit(db)
	bc, _ := NewBlockChain(db, DefaultCacheConfig, &conf, dummy.NewFaker(), vm.Config{}, common.Hash{})
	defer bc.Stop()

	blocks, _, _ := GenerateChain(&conf, genesis, dummy.NewFaker(), db, 32, 10, func(i int, gen *BlockGen) {})

	if _, err := bc.InsertChain(blocks); err != nil {
		t.Fatalf("failed to import blocks: %v", err)
	}
}
