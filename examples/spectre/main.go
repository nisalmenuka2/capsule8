// Copyright 2017 Capsule8, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"runtime"

	"github.com/capsule8/capsule8/pkg/sys/perf"
	"github.com/golang/glog"
)

const minCacheRefs = 10000
const sampleFreq = 10000

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	p, err := perf.NewPerf()
	if err != nil {
		glog.Fatal(err)
	}

	eventAttrs := []*perf.EventAttr{}

	ea := &perf.EventAttr{
		Disabled:   true,
		Type:       perf.PERF_TYPE_HARDWARE,
		Config:     perf.PERF_COUNT_HW_INSTRUCTIONS,
		SampleType: perf.PERF_SAMPLE_TID | perf.PERF_SAMPLE_READ,
		Freq:       true,
		ReadFormat: perf.PERF_FORMAT_GROUP | perf.PERF_FORMAT_ID,
		SampleFreq: sampleFreq,
		Pinned:     true,
		Exclusive:  true,
	}
	eventAttrs = append(eventAttrs, ea)

	llcReadAccess := perf.PERF_COUNT_HW_CACHE_LL |
		(perf.PERF_COUNT_HW_CACHE_OP_READ << 8) |
		(perf.PERF_COUNT_HW_CACHE_RESULT_ACCESS << 16)

	ea = &perf.EventAttr{
		Disabled: true,
		Type:     perf.PERF_TYPE_HW_CACHE,
		Config:   llcReadAccess,
	}
	eventAttrs = append(eventAttrs, ea)

	llcReadMiss := perf.PERF_COUNT_HW_CACHE_LL |
		(perf.PERF_COUNT_HW_CACHE_OP_READ << 8) |
		(perf.PERF_COUNT_HW_CACHE_RESULT_MISS << 16)

	ea = &perf.EventAttr{
		Disabled: true,
		Type:     perf.PERF_TYPE_HW_CACHE,
		Config:   llcReadMiss,
	}
	eventAttrs = append(eventAttrs, ea)

	err = p.Open(eventAttrs)
	if err != nil {
		glog.Fatal(err)
	}

	instructions := make([]uint64, runtime.NumCPU())
	cacheReferences := make([]uint64, runtime.NumCPU())
	cacheMisses := make([]uint64, runtime.NumCPU())

	p.Run(func(sample perf.Sample) {
		var i, cr, cm uint64
		var di, dcr, dcm uint64

		sr, ok := sample.Record.(*perf.SampleRecord)
		if ok {
			for _, v := range sr.V.Values {
				ea := p.EventAttrsByFormatID[v.ID]

				if ea.Config == perf.PERF_COUNT_HW_INSTRUCTIONS {
					i = v.Value
				} else if ea.Config == llcReadAccess {
					cr = v.Value
				} else if ea.Config == llcReadMiss {
					cm = v.Value
				} else {
					glog.Fatalf("Unknown event attr config: %v", ea.Config)
				}

			}

			cpu := sample.CPU

			di = i - instructions[cpu]
			dcr = cr - cacheReferences[cpu]
			dcm = cm - cacheMisses[cpu]

			cacheAccessRate := float32(dcr) / float32(di)
			cacheMissRate := float32(dcm) / float32(dcr)

			// Severities:
			// 0.9
			// 0.95
			// 0.98
			// 0.99

			if dcr > minCacheRefs && cacheMissRate > 0.9 {
				glog.Infof("Potential cache side channel by pid %d / tid %d (cacheAccessRate=%v, cacheRefs=%v, cacheMissRate=%v)",
					sr.Pid, sr.Tid, cacheAccessRate, dcr, cacheMissRate)

			}

			instructions[cpu] = i
			cacheReferences[cpu] = cr
			cacheMisses[cpu] = cm
		}
	})
}
