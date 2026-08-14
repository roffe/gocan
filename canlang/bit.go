package canlang

import (
	lua "github.com/yuin/gopher-lua"
)

// registerBitLib installs a LuaJIT-style `bit` global. Lua 5.1 has no
// bitwise operators, and CAN work is nothing but bit fields. All operations
// are on 32-bit unsigned integers; inputs are normalized modulo 2^32.
func registerBitLib(L *lua.LState) {
	L.SetGlobal("bit", L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"band":    bitFold(^uint32(0), func(a, b uint32) uint32 { return a & b }),
		"bor":     bitFold(0, func(a, b uint32) uint32 { return a | b }),
		"bxor":    bitFold(0, func(a, b uint32) uint32 { return a ^ b }),
		"bnot":    bitNot,
		"lshift":  bitLshift,
		"rshift":  bitRshift,
		"extract": bitExtract,
		"btest":   bitBtest,
	}))
}

// checkUint32 reads a Lua number as a 32-bit unsigned integer, wrapping
// negatives two's-complement style so bit.band(-1, x) == x.
func checkUint32(L *lua.LState, pos int) uint32 {
	return uint32(int64(L.CheckNumber(pos)))
}

// bitFold builds a variadic op like bit.band(x, y, z, ...).
func bitFold(identity uint32, op func(a, b uint32) uint32) lua.LGFunction {
	return func(L *lua.LState) int {
		acc := identity
		for i := 1; i <= L.GetTop(); i++ {
			acc = op(acc, checkUint32(L, i))
		}
		L.Push(lua.LNumber(acc))
		return 1
	}
}

func bitNot(L *lua.LState) int {
	L.Push(lua.LNumber(^checkUint32(L, 1)))
	return 1
}

// shiftLeft implements bit32 shift semantics: displacements of 32 or more
// clear the value, negative displacements shift the other way.
func shiftLeft(x uint32, n int) uint32 {
	switch {
	case n <= -32 || n >= 32:
		return 0
	case n < 0:
		return x >> uint(-n)
	default:
		return x << uint(n)
	}
}

func bitLshift(L *lua.LState) int {
	L.Push(lua.LNumber(shiftLeft(checkUint32(L, 1), L.CheckInt(2))))
	return 1
}

func bitRshift(L *lua.LState) int {
	L.Push(lua.LNumber(shiftLeft(checkUint32(L, 1), -L.CheckInt(2))))
	return 1
}

// bitExtract returns width bits (default 1) starting at bit field, so
// bit.extract(b, 3) reads a flag and bit.extract(b, 4, 8) reads a byte.
func bitExtract(L *lua.LState) int {
	x := checkUint32(L, 1)
	field := L.CheckInt(2)
	width := L.OptInt(3, 1)
	if field < 0 || width < 1 || field+width > 32 {
		L.RaiseError("extract: field %d width %d out of range", field, width)
	}
	mask := uint32(uint64(1)<<uint(width) - 1)
	L.Push(lua.LNumber(x >> uint(field) & mask))
	return 1
}

func bitBtest(L *lua.LState) int {
	acc := ^uint32(0)
	for i := 1; i <= L.GetTop(); i++ {
		acc &= checkUint32(L, i)
	}
	L.Push(lua.LBool(acc != 0))
	return 1
}
