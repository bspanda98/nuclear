// Copyright 2019 The Nuclear Core Authors
// This file is part of Nuclear Core.
//
// Nuclear Core is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Nuclear Core is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with Nuclear Core. If not, see <http://www.gnu.org/licenses/>.

// Nuclear Governance system is the fundamental part of Nuclear Core.

// NOTE: It's not allowed to change the compiler due to byte-to-byte
//       match requirement.
pragma solidity 0.5.16;
//pragma experimental SMTChecker;

import { IGovernedContract } from "./IGovernedContract.sol";
import { IUpgradeProposal } from "./IUpgradeProposal.sol";

interface ISporkRegistry {
    function createUpgradeProposal(
        IGovernedContract _impl,
        uint _period,
        address payable _fee_payer
    )
        external payable
        returns (IUpgradeProposal);

    function consensusGasLimits()
        external view
        returns(uint callGas, uint xferGas);
}
