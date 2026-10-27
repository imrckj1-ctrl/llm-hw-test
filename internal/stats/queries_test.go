package stats

import "testing"

// Dist 分布统计:平均/P50/P95/P99/最短/最长
func TestDist(t *testing.T) {
	d := Dist([]float64{10, 1, 5, 3, 7, 2, 8, 4, 6, 9, 100})
	// 排序后:1 2 3 4 5 6 7 8 9 10 100,n=11
	if d.Min != 1 || d.Max != 100 {
		t.Fatalf("min/max: %v %v", d.Min, d.Max)
	}
	if d.Avg < 14.09 || d.Avg > 14.10 { // 155/11
		t.Fatalf("avg=%v", d.Avg)
	}
	if d.P50 != 6 {
		t.Fatalf("p50=%v", d.P50)
	}
	if d.P95 != 100 || d.P99 != 100 { // ceil(0.95*11)=11, ceil(0.99*11)=11
		t.Fatalf("p95=%v p99=%v", d.P95, d.P99)
	}
	if Dist(nil) != (DistStats{}) {
		t.Fatal("空样本应返回零值")
	}
	one := Dist([]float64{42})
	if one.Avg != 42 || one.P50 != 42 || one.P95 != 42 || one.P99 != 42 || one.Min != 42 || one.Max != 42 {
		t.Fatalf("单样本: %+v", one)
	}
}
