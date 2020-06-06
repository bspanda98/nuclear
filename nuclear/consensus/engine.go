// Copyright 2019 The Nuclear Core Authors
// This file is part of the Nuclear Core library.
//
// The Nuclear Core library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Nuclear Core library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Nuclear Core library. If not, see <http://www.gnu.org/licenses/>.

package consensus

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync/atomic"
	"time"

	"nuclear/core/nuclear/accounts/abi"
	"nuclear/core/nuclear/common"
	eth_consensus "nuclear/core/nuclear/consensus"
	"nuclear/core/nuclear/consensus/misc"
	"nuclear/core/nuclear/core"
	"nuclear/core/nuclear/core/state"
	"nuclear/core/nuclear/core/types"
	"nuclear/core/nuclear/core/vm"
	"nuclear/core/nuclear/crypto"
	"nuclear/core/nuclear/ethdb"
	"nuclear/core/nuclear/log"
	"nuclear/core/nuclear/params"
	"nuclear/core/nuclear/rlp"
	"nuclear/core/nuclear/rpc"

	lru "github.com/hashicorp/golang-lru"
	"golang.org/x/crypto/sha3"

	energi_abi "nuclear/core/nuclear/energi/abi"
	energi_params "nuclear/core/nuclear/energi/params"
)

var (
	sealLen   = 65
	uncleHash = types.CalcUncleHash(nil)

	errMissingSig = errors.New("Signature is missing")
	errInvalidSig = errors.New("Invalid signature")

	errBlacklistedCoinbase = errors.New("Blacklisted coinbase")
)

type ChainReader = eth_consensus.ChainReader
type AccountsFn func() []common.Address
type SignerFn func(common.Address, []byte) ([]byte, error)
type PeerCountFn func() int
type IsMiningFn func() bool
type DiffFn func(ChainReader, uint64, *types.Header, *timeTarget) *big.Int

type Nuclear struct {
	// Atomic alignment to 64-bit
	nonceCap uint64

	// The rest
	config       *params.NuclearConfig
	db           ethdb.Database
	rewardAbi    abi.ABI
	dposAbi      abi.ABI
	blacklistAbi abi.ABI
	sporkAbi     abi.ABI
	mnregAbi     abi.ABI
	treasuryAbi  abi.ABI
	systemFaucet common.Address
	xferGas      uint64
	callGas      uint64
	unlimitedGas uint64
	signerFn     SignerFn
	accountsFn   AccountsFn
	peerCountFn  PeerCountFn
	isMiningFn   IsMiningFn
	diffFn       DiffFn
	testing      bool
	now          func() uint64
	knownStakes  KnownStakes
	nextKSPurge  uint64
	txhashMap    *lru.Cache
}

func New(config *params.NuclearConfig, db ethdb.Database) *Nuclear {
	reward_abi, err := abi.JSON(strings.NewReader(energi_abi.IBlockRewardABI))
	if err != nil {
		panic(err)
		return nil
	}

	dpos_abi, err := abi.JSON(strings.NewReader(energi_abi.IDelegatedPoSABI))
	if err != nil {
		panic(err)
		return nil
	}

	blacklist_abi, err := abi.JSON(strings.NewReader(energi_abi.IBlacklistRegistryABI))
	if err != nil {
		panic(err)
		return nil
	}

	spork_abi, err := abi.JSON(strings.NewReader(energi_abi.ISporkRegistryABI))
	if err != nil {
		panic(err)
		return nil
	}

	mngreg_abi, err := abi.JSON(strings.NewReader(energi_abi.IMasternodeRegistryV2ABI))
	if err != nil {
		panic(err)
		return nil
	}

	treasury_abi, err := abi.JSON(strings.NewReader(energi_abi.ITreasuryABI))
	if err != nil {
		panic(err)
		return nil
	}

	txhashMap, err := lru.New(8)
	if err != nil {
		panic(err)
		return nil
	}

	return &Nuclear{
		config:       config,
		db:           db,
		rewardAbi:    reward_abi,
		dposAbi:      dpos_abi,
		blacklistAbi: blacklist_abi,
		sporkAbi:     spork_abi,
		mnregAbi:     mngreg_abi,
		treasuryAbi:  treasury_abi,
		systemFaucet: energi_params.Nuclear_SystemFaucet,
		xferGas:      0,
		callGas:      30000,
		unlimitedGas: energi_params.UnlimitedGas,
		diffFn:       calcPoSDifficultyV1,
		now:          func() uint64 { return uint64(time.Now().Unix()) },
		nextKSPurge:  0,
		txhashMap:    txhashMap,

		accountsFn:  func() []common.Address { return nil },
		peerCountFn: func() int { return 0 },
		isMiningFn:  func() bool { return false },
	}
}

func (e *Nuclear) createEVM(
	msg types.Message,
	chain ChainReader,
	header *types.Header,
	statedb *state.StateDB,
) *vm.EVM {
	vmc := &vm.Config{}

	if bc, ok := chain.(*core.BlockChain); ok {
		vmc = bc.GetVMConfig()
	}

	// Only From() is used by fact
	ctx := core.NewEVMContext(msg, header, chain.(core.ChainContext), &header.Coinbase)
	ctx.GasLimit = e.xferGas
	return vm.NewEVM(ctx, statedb, chain.Config(), *vmc)
}

// Author retrieves the Ethereum address of the account that minted the given
// block, which may be different from the header's coinbase if a consensus
// engine is based on signatures.
func (e *Nuclear) Author(header *types.Header) (common.Address, error) {
	return common.Address{}, nil
}

// VerifyHeader checks whether a header conforms to the consensus rules of a
// given engine. Verifying the seal may be done optionally here, or explicitly
// via the VerifySeal method.
func (e *Nuclear) VerifyHeader(chain ChainReader, header *types.Header, seal bool) error {
	var err error
	is_migration := header.IsGen2Migration()

	// Ensure that the header's extra-data section is of a reasonable size
	if uint64(len(header.Extra)) > params.MaximumExtraDataSize && !is_migration {
		return fmt.Errorf("extra-data too long: %d > %d",
			len(header.Extra), params.MaximumExtraDataSize)
	}

	// A special Migration block #1
	if is_migration && (header.Coinbase != energi_params.Nuclear_MigrationContract) {
		log.Error("PoS migration mismatch",
			"signer", header.Coinbase,
			"required", energi_params.Nuclear_MigrationContract)
		return errors.New("Invalid Migration")
	}

	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)

	if parent == nil {
		if header.Number.Cmp(common.Big0) != 0 {
			log.Trace("Not found parent", "number", header.Number,
				"hash", header.Hash(), "parent", header.ParentHash)
			return eth_consensus.ErrUnknownAncestor
		}

		return nil
	}

	time_target := e.calcTimeTarget(chain, parent)
	err = e.checkTime(header, time_target)
	if err != nil {
		return err
	}

	modifier := e.calcPoSModifier(chain, header.Time, parent)
	if header.MixDigest != modifier {
		return fmt.Errorf("invalid modifier: have %v, want %v",
			header.MixDigest, modifier)
	}

	difficulty := e.calcPoSDifficulty(chain, header.Time, parent, time_target)

	if header.Difficulty.Cmp(difficulty) != 0 {
		return fmt.Errorf("invalid difficulty: have %v, want %v",
			header.Difficulty, difficulty)
	}

	cap := uint64(0x7fffffffffffffff)
	if header.GasLimit > cap {
		return fmt.Errorf("invalid gasLimit: have %v, max %v",
			header.GasLimit, cap)
	}

	// Verify that the gasUsed is <= gasLimit, except for migration
	if (header.GasUsed > header.GasLimit) && !is_migration {
		return fmt.Errorf("invalid gasUsed: have %d, gasLimit %d",
			header.GasUsed, header.GasLimit)
	}

	// Verify that the gas limit remains within allowed bounds
	diff := int64(parent.GasLimit) - int64(header.GasLimit)
	if diff < 0 {
		diff *= -1
	}
	limit := parent.GasLimit / params.GasLimitBoundDivisor

	if (uint64(diff) >= limit) && !is_migration && !parent.IsGen2Migration() {
		return fmt.Errorf("invalid gas limit: have %d, want %d += %d",
			header.GasLimit, parent.GasLimit, limit)
	}

	if header.GasLimit < params.MinGasLimit {
		return fmt.Errorf("invalid gas limit: have %d, minimum %d",
			header.GasLimit, params.MinGasLimit)
	}

	// Verify that the block number is parent's +1
	if diff := new(big.Int).Sub(header.Number, parent.Number); diff.Cmp(big.NewInt(1)) != 0 {
		return eth_consensus.ErrInvalidNumber
	}

	// We skip checks only where full previous meturity period state is required.
	if seal {
		// Verify the engine specific seal securing the block
		err = e.VerifySeal(chain, header)
		if err != nil {
			return err
		}

		err = e.verifyPoSHash(chain, header)
		if err != nil {
			return err
		}
	}

	if err = misc.VerifyForkHashes(chain.Config(), header, false); err != nil {
		return err
	}

	// DoS protection
	if seal && chain.GetHeader(header.Hash(), header.Number.Uint64()) == nil {
		if err = e.checkDoS(chain, header, parent); err != nil {
			return err
		}
	}

	return nil
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
// concurrently. The method returns a quit channel to abort the operations and
// a results channel to retrieve the async verifications (the order is that of
// the input slice).
func (e *Nuclear) VerifyHeaders(
	chain ChainReader, headers []*types.Header, seals []bool,
) (
	chan<- struct{}, <-chan error, chan<- bool,
) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))
	ready := make(chan bool, len(headers))

	go func() {
		for i, header := range headers {
			// NOTE: unlike Ethash with DAG, there is little sense of this
			//       batch async routine overhead
			select {
			case <-abort:
				return
			case <-ready:
			}

			err := e.VerifyHeader(chain, header, seals[i])

			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()

	return abort, results, ready
}

// VerifyUncles verifies that the given block's uncles conform to the consensus
// rules of a given engine.
func (e *Nuclear) VerifyUncles(chain ChainReader, block *types.Block) error {
	if len(block.Uncles()) > 0 {
		return errors.New("uncles not allowed")
	}
	return nil
}

// VerifySeal checks whether the crypto seal on a header is valid according to
// the consensus rules of the given engine.
func (e *Nuclear) VerifySeal(chain ChainReader, header *types.Header) error {
	parent_number := header.Number.Uint64() - 1
	blockst := chain.CalculateBlockState(header.ParentHash, parent_number)
	if blockst == nil {
		log.Error("PoS state root failure", "header", header.ParentHash)
		return eth_consensus.ErrMissingState
	}

	// DBL-8: blacklist block generation
	if core.IsBlacklisted(blockst, header.Coinbase) {
		log.Debug("Blacklisted Coinbase", "addr", header.Coinbase)
		return errBlacklistedCoinbase
	}

	// Retrieve the signature from the header extra-data
	if len(header.Signature) != sealLen {
		return errMissingSig
	}

	sighash := e.SignatureHash(header)
	log.Trace("PoS verify signature hash", "sighash", sighash)

	r := new(big.Int).SetBytes(header.Signature[:32])
	s := new(big.Int).SetBytes(header.Signature[32:64])
	v := header.Signature[64]

	if !crypto.ValidateSignatureValues(v, r, s, true) {
		return types.ErrInvalidSig
	}

	pubkey, err := crypto.Ecrecover(sighash.Bytes(), header.Signature)
	if err != nil {
		return err
	}

	var addr common.Address
	copy(addr[:], crypto.Keccak256(pubkey[1:])[12:])

	if addr != header.Coinbase {
		// POS-5: Delegated PoS
		//--
		parent := chain.GetHeader(header.ParentHash, parent_number)
		if parent == nil {
			return eth_consensus.ErrUnknownAncestor
		}

		if blockst.GetCodeSize(header.Coinbase) > 0 {
			signerData, err := e.dposAbi.Pack("signerAddress")
			if err != nil {
				log.Error("Fail to prepare signerAddress() call", "err", err)
				return err
			}

			msg := types.NewMessage(
				e.systemFaucet,
				&header.Coinbase,
				0,
				common.Big0,
				e.callGas,
				common.Big0,
				signerData,
				false,
			)

			rev_id := blockst.Snapshot()
			evm := e.createEVM(msg, chain, parent, blockst)
			gp := core.GasPool(e.callGas)
			output, _, _, err := core.ApplyMessage(evm, msg, &gp)
			blockst.RevertToSnapshot(rev_id)
			if err != nil {
				log.Trace("Fail to get signerAddress()", "err", err)
				return err
			}

			//
			signer := common.Address{}
			err = e.dposAbi.Unpack(&signer, "signerAddress", output)
			if err != nil {
				log.Error("Failed to unpack signerAddress() call", "err", err)
				return err
			}

			if signer == addr {
				return nil
			}

			log.Trace("PoS seal compare", "addr", addr, "signer", signer)
		} else {
			log.Trace("PoS seal compare", "addr", addr, "coinbase", header.Coinbase)
		}

		return errInvalidSig
	}

	return nil
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
func (e *Nuclear) Prepare(chain ChainReader, header *types.Header) error {
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)

	if parent == nil {
		log.Error("Fail to find parent", "header", header)
		return eth_consensus.ErrUnknownAncestor
	}

	_, err := e.posPrepare(chain, header, parent)
	return err
}

func (e *Nuclear) posPrepare(
	chain ChainReader,
	header *types.Header,
	parent *types.Header,
) (time_target *timeTarget, err error) {
	// Clear field to be set in mining
	header.Coinbase = common.Address{}
	header.Nonce = types.BlockNonce{}

	time_target = e.calcTimeTarget(chain, parent)

	err = e.enforceTime(header, time_target)

	// Repurpose the MixDigest field
	header.MixDigest = e.calcPoSModifier(chain, header.Time, parent)

	// Diff
	header.Difficulty = e.calcPoSDifficulty(chain, header.Time, parent, time_target)

	return time_target, err
}

// Finalize runs any post-transaction state modifications (e.g. block rewards)
// and assembles the final block.
// Note: The block header and state database might be updated to reflect any
// consensus rules that happen at finalization (e.g. block rewards).
func (e *Nuclear) Finalize(
	chain ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction,
	uncles []*types.Header, receipts []*types.Receipt,
) (*types.Block, []*types.Receipt, error) {
	ctxs := types.Transactions{}

	for i := (len(txs) - 1); i >= 0; i-- {
		if !txs[i].IsConsensus() {
			i++
			ctxs = txs[i:]
			txs = txs[:i]
			break
		} else if i == 0 {
			ctxs = txs[:]
			txs = txs[:0]
			break
		}
	}

	block, receipts, err := e.finalize(chain, header, state, txs, receipts)
	if err != nil {
		return nil, nil, err
	}

	ntxs := block.Transactions()[len(txs):]
	if len(ntxs) != len(ctxs) {
		log.Trace("Consensus TX length mismatch", "ntxs", len(ntxs), "ctxs", len(ctxs))
		return nil, nil, eth_consensus.ErrInvalidConsensusTx
	}
	for i, tx := range ntxs {
		if tx.Hash() != ctxs[i].Hash() {
			log.Trace("Consensus TX hash mismatch", "pos", i, "ctx", ctxs[i].Hash(), "ntx", tx.Hash())
			return nil, nil, eth_consensus.ErrInvalidConsensusTx
		}
	}

	return block, receipts, err
}

func (e *Nuclear) finalize(
	chain ChainReader, header *types.Header, state *state.StateDB,
	txs []*types.Transaction, receipts []*types.Receipt,
) (*types.Block, []*types.Receipt, error) {
	var err error

	// Do not finalize too early in mining
	if (header.Coinbase != common.Address{}) {
		txs, receipts, err = e.govFinalize(chain, header, state, txs, receipts)
	}

	header.UncleHash = uncleHash

	return types.NewBlock(header, txs, nil, receipts), receipts, err
}

func (e *Nuclear) govFinalize(
	chain ChainReader,
	header *types.Header,
	state *state.StateDB,
	txs types.Transactions,
	receipts types.Receipts,
) (types.Transactions, types.Receipts, error) {
	err := e.processConsensusGasLimits(chain, header, state)
	if err == nil {
		txs, receipts, err = e.processBlockRewards(chain, header, state, txs, receipts)
	}
	if err == nil {
		err = e.processMasternodes(chain, header, state)
	}
	if err == nil {
		err = e.processBlacklists(chain, header, state)
	}
	if err == nil {
		txs, receipts, err = e.processDrainable(chain, header, state, txs, receipts)
	}
	if err == nil {
		err = e.finalizeMigration(chain, header, state, txs)
	}
	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	return txs, receipts, err
}

// Seal generates a new sealing request for the given input block and pushes
// the result into the given channel.
//
// Note, the method returns immediately and will send the result async. More
// than one result may also be returned depending on the consensus algorithm.
func (e *Nuclear) Seal(
	chain ChainReader,
	block *types.Block,
	results chan<- *eth_consensus.SealResult,
	stop <-chan struct{},
) (err error) {
	go func() {
		header := block.Header()
		txhash := header.TxHash
		result := eth_consensus.NewSealResult(block, nil, nil)

		if header.Number.Cmp(common.Big0) != 0 {
			success, err := e.mine(chain, header, stop)

			// NOTE: due to the fact that PoS mining may change Coinbase
			//       it is required to reprocess all transaction with correct
			//       state of the block (input parameters). This is essential
			//       for consensus and correct distribution of gas.
			if success && err == nil {
				result, err = e.recreateBlock(chain, header, block.Transactions())
			}

			if err != nil {
				log.Error("PoS miner error", "err", err)
				success = false
			}

			if !success {
				select {
				case results <- eth_consensus.NewSealResult(nil, nil, nil):
				default:
				}
				return
			}

			header = result.Block.Header()
		}

		sighash := e.SignatureHash(header)
		log.Trace("PoS seal hash", "sighash", sighash)

		header.Signature, err = e.signerFn(header.Coinbase, sighash.Bytes())
		if err != nil {
			log.Error("PoS miner error", "err", err)
			return
		}

		result.Block = result.Block.WithSeal(header)
		e.txhashMap.Add(header.TxHash, txhash)

		select {
		case results <- result:
			log.Info("PoS seal has submitted solution", "block", result.Block.Hash())
		default:
			log.Warn("PoS seal is not read by miner", "sealhash", e.SealHash(header))
		}
	}()

	return nil
}

func (e *Nuclear) recreateBlock(
	chain ChainReader,
	header *types.Header,
	txs types.Transactions,
) (
	result *eth_consensus.SealResult, err error,
) {
	var (
		usedGas = new(uint64)
		gp      = new(core.GasPool).AddGas(header.GasLimit)

		bc *core.BlockChain
		ok bool
	)

	blstate := chain.CalculateBlockState(header.ParentHash, header.Number.Uint64()-1)
	if err != nil {
		return nil, eth_consensus.ErrUnknownAncestor
	}

	vmc := &vm.Config{}
	if bc, ok = chain.(*core.BlockChain); ok {
		vmc = bc.GetVMConfig()
	}

	receipts := make(types.Receipts, 0, len(txs))

	for i, tx := range txs {
		blstate.Prepare(tx.Hash(), common.Hash{}, i)
		receipt, _, err := core.ApplyTransaction(
			chain.Config(), bc, nil,
			gp, blstate, header, tx, usedGas, *vmc)
		if err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}

	header.GasUsed = *usedGas
	header.Bloom = types.Bloom{}

	block, receipts, err := e.finalize(chain, header, blstate, txs, receipts)
	return eth_consensus.NewSealResult(block, blstate, receipts), err
}

// SealHash returns the hash of a block prior to it being sealed.
func (e *Nuclear) SealHash(header *types.Header) (hash common.Hash) {
	hasher := sha3.NewLegacyKeccak256()

	txhash := header.TxHash

	if item, ok := e.txhashMap.Get(txhash); ok {
		txhash = item.(common.Hash)
	}

	rlp.Encode(hasher, []interface{}{
		// NOTE: commented parts are part of "mining" process
		header.ParentHash,
		header.UncleHash,
		//header.Coinbase,
		//header.Root,
		txhash,
		//header.ReceiptHash,
		//header.Bloom,
		//header.Difficulty,
		header.Number,
		header.GasLimit,
		//header.GasUsed,
		//header.Time,
		//header.Extra,
		//header.MixDigest,
		//header.Nonce,
		//header.Signature,
	})
	hasher.Sum(hash[:0])
	return hash
}

func (e *Nuclear) SignatureHash(header *types.Header) (hash common.Hash) {
	hasher := sha3.NewLegacyKeccak256()

	rlp.Encode(hasher, []interface{}{
		header.ParentHash,
		header.UncleHash,
		header.Coinbase,
		header.Root,
		header.TxHash,
		header.ReceiptHash,
		header.Bloom,
		header.Difficulty,
		header.Number,
		header.GasLimit,
		header.GasUsed,
		header.Time,
		header.Extra,
		header.MixDigest,
		header.Nonce,
		//header.Signature,
	})
	hasher.Sum(hash[:0])
	return hash
}

func (e *Nuclear) SetMinerNonceCap(nonceCap uint64) {
	atomic.StoreUint64(&e.nonceCap, nonceCap)
}
func (e *Nuclear) GetMinerNonceCap() uint64 {
	return atomic.LoadUint64(&e.nonceCap)
}
func (e *Nuclear) SetMinerCB(
	accountsFn AccountsFn,
	signerFn SignerFn,
	peerCountFn PeerCountFn,
	isMiningFn IsMiningFn,
) {
	if e.signerFn != nil {
		panic("Callbacks must be set only once!")
	}

	e.signerFn = signerFn
	e.accountsFn = accountsFn
	e.peerCountFn = peerCountFn
	e.isMiningFn = isMiningFn
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
// that a new block should have.
func (e *Nuclear) CalcDifficulty(chain ChainReader, time uint64, parent *types.Header) *big.Int {
	time_target := e.calcTimeTarget(chain, parent)
	return e.calcPoSDifficulty(chain, time, parent, time_target)
}

// APIs returns the RPC APIs this consensus engine provides.
func (e *Nuclear) APIs(chain ChainReader) []rpc.API {
	return []rpc.API{
		{
			Namespace: "miner",
			Version:   "1.0",
			Service:   NewEngineAPI(chain, e),
			Public:    true,
		},
	}
}

// Close terminates any background threads maintained by the consensus engine.
func (e *Nuclear) Close() error {
	return nil
}

func (e *Nuclear) processConsensusGasLimits(
	chain ChainReader,
	header *types.Header,
	state *state.StateDB,
) error {
	callData, err := e.sporkAbi.Pack("consensusGasLimits")
	if err != nil {
		log.Error("Fail to prepare consensusGasLimits() call", "err", err)
		return err
	}

	// consensusGasLimits()
	msg := types.NewMessage(
		e.systemFaucet,
		&energi_params.Nuclear_SporkRegistry,
		0,
		common.Big0,
		e.callGas,
		common.Big0,
		callData,
		false,
	)
	rev_id := state.Snapshot()
	evm := e.createEVM(msg, chain, header, state)
	gp := core.GasPool(e.callGas)
	output, _, _, err := core.ApplyMessage(evm, msg, &gp)
	state.RevertToSnapshot(rev_id)
	if err != nil {
		log.Error("Failed in consensusGasLimits() call", "err", err)
		return err
	}

	//
	ret := new(struct {
		CallGas *big.Int
		XferGas *big.Int
	})
	err = e.sporkAbi.Unpack(ret, "consensusGasLimits", output)
	if err != nil {
		log.Error("Failed to unpack consensusGasLimits() call", "err", err)
		return err
	}

	e.callGas = ret.CallGas.Uint64()
	e.xferGas = ret.XferGas.Uint64()
	log.Trace("Consensus Gas", "call", e.callGas, "xfer", e.xferGas)

	return nil
}
