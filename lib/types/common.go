package types

import (
	"fmt"
	"math/big"
	"strings"
	"time"
)

const (
	KiB = 1024
	MiB = 1048576
	GiB = 1073741824
	TiB = 1099511627776

	KB = 1e3
	MB = 1e6
	GB = 1e9
	TB = 1e12

	Day    = 86400
	Hour   = 3600
	Minute = 60

	Wei   = 1
	GWei  = 1e9
	Token = 1e18
)

const (
	// show time format
	TimeShow = time.RFC3339

	// DefaultDuration is default store days: 100 days
	DefaultDuration = 100 * Day // second

	// DefaultCapacity is provider deposit capacity, 1TB
	DefaultCapacity = TiB

	// 1token = 10^18 wei
	// samely Dollar is 10^18 WeiDollar

	// Memo2Dollar is default Memo token Price, 1 token = 0.01 dollar
	Memo2Dollar float64 = 0.01

	// DefaultReadPrice is read price 0.00002 $/GB(0.25 rmb-0.5rmb/GB in aliyun oss)
	DefaultReadPrice = 2_000_000 // weiDollar per Byte
	// DefaultStorePrice is stored price 3$/TB*Month (33 rmb/TB*Month in aliyun oss)
	DefaultStorePrice = 1 // weiDollar per Byte*second
	// ProviderDeposit is provider deposit price, 3 dollar/TB
	ProviderDeposit = 3_000_000 // weiDollar per Byte
	// KeeperDeposit is keeper deposit； 0.01 dollar for now
	KeeperDeposit = Token
)

type DiskStats struct {
	Total uint64 `json:"all"`
	Used  uint64 `json:"used"`
	Free  uint64 `json:"free"`
}

// FormatBytes convert bytes to human readable string. Like 2 MiB, 64.2 KiB, 52 B
func FormatBytes(i uint64) string {
	switch {
	case i >= TiB:
		return fmt.Sprintf("%.02f TiB", float64(i)/TiB)
	case i >= GiB:
		return fmt.Sprintf("%.02f GiB", float64(i)/GiB)
	case i >= MiB:
		return fmt.Sprintf("%.02f MiB", float64(i)/MiB)
	case i >= KiB:
		return fmt.Sprintf("%.02f KiB", float64(i)/KiB)
	default:
		return fmt.Sprintf("%d B", i)
	}
}

// FormatBytesDec Convert bytes to base-10 human readable string. Like 2 MB, 64.2 KB, 52 B
func FormatBytesDec(i int64) string {
	switch {
	case i >= TB:
		return fmt.Sprintf("%.02f TB", float64(i)/TB)
	case i >= GB:
		return fmt.Sprintf("%.02f GB", float64(i)/GB)
	case i >= MB:
		return fmt.Sprintf("%.02f MB", float64(i)/MB)
	case i >= KB:
		return fmt.Sprintf("%.02f KB", float64(i)/KB)
	default:
		return fmt.Sprintf("%d B", i)
	}
}

func FormatMemo(i *big.Int) string {
	f := new(big.Float).SetInt(i)
	res, _ := f.Float64()
	switch {
	case res >= Token:
		return fmt.Sprintf("%.02f Memo", res/Token)
	case res >= GWei:
		return fmt.Sprintf("%.02f NanoMemo", res/GWei)
	default:
		return fmt.Sprintf("%d AttoMemo", i.Int64())
	}
}

func FormatEth(i *big.Int) string {
	f := new(big.Float).SetInt(i)
	res, _ := f.Float64()
	switch {
	case res >= Token:
		return fmt.Sprintf("%.02f Eth", res/Token)
	case res >= GWei:
		return fmt.Sprintf("%.02f Gwei", res/GWei)
	default:
		return fmt.Sprintf("%d Wei", i.Int64())
	}
}

func FormatWeiDollar(i *big.Int) string {
	f := new(big.Float).SetInt(i)
	res, _ := f.Float64()
	switch {
	case res >= Token:
		return fmt.Sprintf("%.02f Dollar (For now, 1 Dollar = 100 Token)", res/Token)
	case res >= GWei:
		return fmt.Sprintf("%.02f GweiDollar (For now, 1 GweiDollar = 100 Gwei)", res/GWei)
	default:
		return fmt.Sprintf("%f WeiDollar (For now, 1 WeiDollar = 100 Wei)", res)
	}
}

func FormatStorePrice(i *big.Int) string {
	f := new(big.Float).SetInt(i)
	f.Mul(f, big.NewFloat(float64(30*Day)*float64(TiB)))
	res, _ := f.Float64()
	switch {
	case res >= Token:
		return fmt.Sprintf("%.02f Dollar/(TiB*Month) (For now, 1 Dollar = 100 Token)", res/Token)
	case res >= GWei:
		return fmt.Sprintf("%.02f GweiDollar/(TiB*Month) (For now, 1 GweiDollar = 100 Gwei)", res/GWei)
	default:
		return fmt.Sprintf("%f WeiDollar/(TiB*Month) (For now, 1 WeiDollar = 100 Wei)", res)
	}
}

func FormatReadPrice(i *big.Int) (result string) {
	f := new(big.Float).SetInt(i)
	f.Mul(f, big.NewFloat(float64(GiB)))
	res, _ := f.Float64()
	switch {
	case res >= Token:
		result = fmt.Sprintf("%.02f Dollar/GiB (For now, 1 Dollar = 100 Token)", res/Token)
	case res >= GWei:
		result = fmt.Sprintf("%.02f GweiDollar/GiB (For now, 1 GweiDollar = 100 Gwei)", res/GWei)
	default:
		result = fmt.Sprintf("%f WeiDollar/GiB (For now, 1 WeiDollar = 100 Wei)", res)
	}
	return
}

func ParsetEthValue(s string) (*big.Int, error) {
	if len(s) > 32 {
		return nil, fmt.Errorf("string length too large: %d", len(s))
	}

	suffix := strings.TrimLeft(s, "-.1234567890")
	s = s[:len(s)-len(suffix)]
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, fmt.Errorf("failed to parse %q as a decimal number", s)
	}

	norm := strings.ToLower(strings.TrimSpace(suffix))
	switch norm {
	case "", "token":
		r.Mul(r, big.NewRat(1_000_000_000, 1))
		r.Mul(r, big.NewRat(1_000_000_000, 1))
	case "gwei":
		r.Mul(r, big.NewRat(1_000_000_000, 1))
	case "wei":
	default:
		return nil, fmt.Errorf("unrecognized suffix: %q", suffix)
	}

	return r.Num(), nil
}

func ParsetValue(s string) (*big.Int, error) {
	if len(s) > 32 {
		return nil, fmt.Errorf("string length too large: %d", len(s))
	}

	suffix := strings.TrimLeft(s, "-.1234567890")
	s = s[:len(s)-len(suffix)]
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, fmt.Errorf("failed to parse %q as a decimal number", s)
	}

	norm := strings.ToLower(strings.TrimSpace(suffix))
	switch norm {
	case "", "memo":
		r.Mul(r, big.NewRat(1_000_000_000, 1))
		r.Mul(r, big.NewRat(1_000_000_000, 1))
	case "nanomemo":
		r.Mul(r, big.NewRat(1_000_000_000, 1))
	case "attomemo":
	default:
		return nil, fmt.Errorf("unrecognized suffix: %q", suffix)
	}

	return r.Num(), nil
}
