// Copyright 2019 The Nuclear Core Authors
// Copyright 2018 The go-ethereum Authors
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

// +build !race

package stream

// Provide a flag to reduce the scope of tests when running them
// with race detector. Some of the tests are doing a lot of allocations
// on the heap, and race detector uses much more memory to track them.
const raceTest = false
