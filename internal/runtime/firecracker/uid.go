package firecracker

import (
	"bufio"
	"errors"
	"os"
	"sort"
	"strconv"
	"strings"
)

const firstGuestUID = 200000
const guestUIDLimit = 400000

// reserveHostUID skips subordinate identifiers used by other host workloads.
// The monotonic counter separately prevents reuse across VM generations.
func reserveHostUID(next int, files ...string) (int, error) {
	type idRange struct{ first, end uint64 }
	var ranges []idRange
	for _, path := range files {
		file, err := os.Open(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return 0, err
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			fields := strings.Split(line, ":")
			if len(fields) != 3 || fields[0] == "" {
				file.Close()
				return 0, errors.New("invalid subordinate ID allocation")
			}
			first, firstErr := strconv.ParseUint(fields[1], 10, 32)
			count, countErr := strconv.ParseUint(fields[2], 10, 32)
			if firstErr != nil || countErr != nil || count == 0 || first+count > 1<<32 {
				file.Close()
				return 0, errors.New("invalid subordinate ID range")
			}
			ranges = append(ranges, idRange{first, first + count})
		}
		file.Close()
		if err := scanner.Err(); err != nil {
			return 0, err
		}
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].first < ranges[j].first })
	candidate := uint64(next)
	for _, ids := range ranges {
		if ids.first > candidate {
			break
		}
		candidate = max(candidate, ids.end)
	}
	if candidate < firstGuestUID || candidate >= guestUIDLimit {
		return 0, errors.New("UID pool unavailable")
	}
	return int(candidate), nil
}
