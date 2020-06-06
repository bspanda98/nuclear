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

package params

import (
	"math/big"

	"nuclear/core/nuclear/common"
)

var (
	Nuclear_BlockReward        = common.BigToAddress(big.NewInt(0x300))
	Nuclear_Treasury           = common.BigToAddress(big.NewInt(0x301))
	Nuclear_MasternodeRegistry = common.BigToAddress(big.NewInt(0x302))
	Nuclear_StakerReward       = common.BigToAddress(big.NewInt(0x303))
	Nuclear_BackboneReward     = common.BigToAddress(big.NewInt(0x304))
	Nuclear_SporkRegistry      = common.BigToAddress(big.NewInt(0x305))
	Nuclear_CheckpointRegistry = common.BigToAddress(big.NewInt(0x306))
	Nuclear_BlacklistRegistry  = common.BigToAddress(big.NewInt(0x307))
	Nuclear_MigrationContract  = common.BigToAddress(big.NewInt(0x308))
	Nuclear_MasternodeToken    = common.BigToAddress(big.NewInt(0x309))
	Nuclear_Blacklist          = common.BigToAddress(big.NewInt(0x30A))
	Nuclear_Whitelist          = common.BigToAddress(big.NewInt(0x30B))
	Nuclear_MasternodeList     = common.BigToAddress(big.NewInt(0x30C))

	Nuclear_BlockRewardV1        = common.BigToAddress(big.NewInt(0x310))
	Nuclear_TreasuryV1           = common.BigToAddress(big.NewInt(0x311))
	Nuclear_MasternodeRegistryV1 = common.BigToAddress(big.NewInt(0x312))
	Nuclear_StakerRewardV1       = common.BigToAddress(big.NewInt(0x313))
	Nuclear_BackboneRewardV1     = common.BigToAddress(big.NewInt(0x314))
	Nuclear_SporkRegistryV1      = common.BigToAddress(big.NewInt(0x315))
	Nuclear_CheckpointRegistryV1 = common.BigToAddress(big.NewInt(0x316))
	Nuclear_BlacklistRegistryV1  = common.BigToAddress(big.NewInt(0x317))
	Nuclear_CompensationFundV1   = common.BigToAddress(big.NewInt(0x318))
	Nuclear_MasternodeTokenV1    = common.BigToAddress(big.NewInt(0x319))

	Nuclear_SystemFaucet = common.BigToAddress(big.NewInt(0x320))
	Nuclear_Ephemeral    = common.HexToAddress("0x457068656d6572616c")

	// NOTE: this is NOT very safe, but it optimizes significantly
	Storage_ProxyImpl = common.BigToHash(big.NewInt(0x01))
)
