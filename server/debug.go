// SPDX-FileCopyrightText: 2021 Softbear, Inc.
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"bytes"
	"fmt"
	"github.com/SoftbearStudios/mk48/server/terrain"
	"github.com/SoftbearStudios/mk48/server/world"
	"image/png"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Debug prints debugging info to console and tmp files.
func (h *Hub) Debug() {
	fmt.Printf("Debug [%v] %s\n", time.Now().Format(time.UnixDate), h.cloud)
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	fmt.Printf(" - memstats: %dM/%dM\n", stats.HeapInuse/1e6, stats.NextGC/1e6)

	var (
		botCount          int
		realPlayerClients []Client
		fps               float32
		fpsCount          int // Can be less than len(realPlayers) for players that haven't sent a trace yet
	)

	for client := h.clients.First; client != nil; client = client.Data().Next {
		if client.Bot() {
			botCount++
		} else {
			realPlayerClients = append(realPlayerClients, client)
		}
	}

	sort.Slice(realPlayerClients, func(i, j int) bool {
		a, b := &realPlayerClients[i].Data().Player, &realPlayerClients[j].Data().Player
		if (a.EntityID == world.EntityIDInvalid) != (b.EntityID == world.EntityIDInvalid) {
			return a.EntityID == world.EntityIDInvalid
		}
		if a.TeamID != b.TeamID {
			return a.TeamID < b.TeamID
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Score < b.Score
	})

	fmt.Printf(" - clients: %d, bots: %d, teams: %d, world radius: %.02f\n", len(realPlayerClients), botCount, len(h.teams), h.worldRadius)

	h.ipMu.RLock()
	fmt.Println(" - ip conns:", h.ipConns)
	h.ipMu.RUnlock()

	for _, client := range realPlayerClients {
		player := &client.Data().Player
		if player.FPS != 0 {
			fps += player.FPS
			fpsCount++
		}

		fmt.Printf("   - %s", player.String())
		if player.EntityID == world.EntityIDInvalid {
			fmt.Print(" {spawning}")
		}
		if sc, ok := client.(*SocketClient); ok && sc.ipStr != "" {
			fmt.Printf(" <%s>", sc.ipStr)
		}
		fmt.Println()
	}

	if fpsCount > 0 {
		// Average
		fps /= float32(fpsCount)
		fmt.Printf(" - fps: %.1f\n", fps)
	}

	fmt.Print(" - ")
	h.terrain.Debug()

	fmt.Print(" - ")
	h.world.Debug()

	// Function benchmarks
	var totalDuration time.Duration

	fmt.Print(" - ")
	for i := range h.funcBenches {
		bench := &h.funcBenches[i]

		duration := bench.reset()
		totalDuration += duration

		fmt.Print(bench.name, ": ", duration, ", ")
	}
	fmt.Println("total:", totalDuration)

	// Count entities
	entityTypeCounts := make([]int, world.EntityTypeCount)
	h.world.ForEntities(func(entity *world.Entity) (_, _ bool) {
		entityTypeCounts[entity.EntityType]++
		return
	})

	_ = AppendLog("/tmp/mk48.log", []interface{}{
		unixMillis(),
		len(realPlayerClients),
		botCount,
		fps,
	})

	var countBuf strings.Builder
	countBuf.Grow(128)
	// Temp buf for entityType strings and integers
	tmpBuf := make([]byte, 0, 16)

	first := true
	countBuf.WriteByte('{')

	for i, c := range entityTypeCounts {
		if c == 0 {
			continue
		}
		if !first {
			countBuf.WriteByte(',')
		} else {
			first = false
		}

		entityType := world.EntityType(i)

		// ex: "fairmileD": 100
		countBuf.WriteByte('"')
		countBuf.Write(entityType.AppendText(tmpBuf))
		countBuf.WriteString("\":")
		countBuf.Write(strconv.AppendInt(tmpBuf, int64(c), 10))
	}

	countBuf.WriteByte('}')

	_ = AppendLog("/tmp/mk48-entities.log", []interface{}{
		unixMillis(),
		countBuf.String(),
	})
}

// Saves a snapshot of the terrain to a tmp directory
func (h *Hub) SnapshotTerrain() {
	if h.cloud == nil {
		return
	}

	defer h.timeFunction("snapshot", time.Now())

	img := h.terrain.Render(terrain.Size / 4)
	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	if err != nil {
		return
	}
	_ = h.cloud.UploadTerrainSnapshot(buf.Bytes())
}

// Logs and saves the panic info, exits
func DebugExit() {
	if r := recover(); r != nil {
		stack := debug.Stack()

		var buf bytes.Buffer
		fmt.Fprintf(&buf, "panic: %v\n", r)
		fmt.Fprintf(&buf, "time: %s\n", time.Now())
		fmt.Fprintln(&buf)
		fmt.Fprintln(&buf, string(stack))

		b := buf.Bytes()

		// Prints to stderr (hopefully unbuffered)
		println(string(b))

		name := fmt.Sprintf("/tmp/mk48-crash-%d.txt", unixMillis())
		err := os.WriteFile(name, b, 0644)
		if err == nil {
			fmt.Println("Wrote to", name)
		} else {
			fmt.Printf("Error writing to %s: %v\n", name, err)
		}

		// Give some time for systemd to record output without cutting it off
		time.Sleep(time.Second)
	}

	os.Exit(1)
}

// funcBench is a benchmark of a core function.
type funcBench struct {
	name     string
	duration time.Duration
	runs     int
}

// reset resets the benchmark and returns the average duration
func (bench *funcBench) reset() time.Duration {
	if bench.runs == 0 {
		return 0
	}
	average := bench.duration / time.Duration(bench.runs)
	bench.duration = 0
	bench.runs = 0
	return average
}

// timeFunction times a function.
// defer timeFunction("name", time.Now())
func (h *Hub) timeFunction(name string, start time.Time) {
	end := time.Now()

	var bench *funcBench
	for i := range h.funcBenches {
		b := &h.funcBenches[i]
		if name == b.name {
			bench = b
			break
		}
	}

	if bench == nil {
		h.funcBenches = append(h.funcBenches, funcBench{name: name})
		bench = &h.funcBenches[len(h.funcBenches)-1]
	}

	bench.duration += end.Sub(start)
	bench.runs++
}
