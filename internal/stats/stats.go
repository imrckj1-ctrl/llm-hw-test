// Package stats 中位数/分位/窗口聚合/能效/衰减计算(附录 A)
package stats

import "sort"

// Median 中位数
func Median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// Percentile 线性插值分位数(§6.4 P99)
func Percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	if len(s) == 1 {
		return s[0]
	}
	rank := p / 100 * float64(len(s)-1)
	lo := int(rank)
	hi := lo + 1
	if hi >= len(s) {
		return s[len(s)-1]
	}
	frac := rank - float64(lo)
	return s[lo] + frac*(s[hi]-s[lo])
}

func Mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func Max(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	m := xs[0]
	for _, x := range xs[1:] {
		if x > m {
			m = x
		}
	}
	return m
}

func Min(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	m := xs[0]
	for _, x := range xs[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

// Degradation 性能衰减 = (第一窗口速度 − 最末窗口速度) ÷ 第一窗口速度 × 100%(§6.5)
func Degradation(first, last float64) float64 {
	if first <= 0 {
		return 0
	}
	return (first - last) / first * 100
}
