package utils

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"go.uber.org/zap"
)

const resourceThreshold = 80.0

// WaitForResources blocks until CPU và RAM đều dưới 80%.
// Trả về false nếu ctx bị huỷ trong lúc chờ.
func WaitForResources(ctx context.Context, log *zap.Logger, tag string) bool {
	for {
		cpuPct, ramPct, err := systemUsage()
		if err != nil {
			return true // không đo được, cho tiếp tục
		}

		if cpuPct < resourceThreshold && ramPct < resourceThreshold {
			return true
		}

		log.Sugar().Warnf("%s High resources CPU=%.1f%% RAM=%.1f%% — waiting 15s...", tag, cpuPct, ramPct)
		select {
		case <-time.After(15 * time.Second):
		case <-ctx.Done():
			return false
		}
	}
}

func systemUsage() (cpuPct, ramPct float64, err error) {
	pcts, err := cpu.Percent(500*time.Millisecond, false)
	if err != nil || len(pcts) == 0 {
		return 0, 0, err
	}
	vm, err := mem.VirtualMemory()
	if err != nil {
		return 0, 0, err
	}
	return pcts[0], vm.UsedPercent, nil
}
