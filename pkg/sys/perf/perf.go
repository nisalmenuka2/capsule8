package perf

import (
	"bytes"
	"runtime"

	"github.com/golang/glog"
	"golang.org/x/sys/unix"
)

type Perf struct {
	eventAttrs []*EventAttr
	options    eventMonitorOptions

	fds      []int
	groupFds []int

	EventAttrsByFormatID eventAttrMap
	ringBuffers          []*ringBuffer
	onSample             func(Sample)
}

func NewPerf(options ...EventMonitorOption) (*Perf, error) {
	p := Perf{}

	for _, o := range options {
		o(&p.options)
	}

	return &p, nil
}

func (p *Perf) Open(eas []*EventAttr) error {
	ncpu := runtime.NumCPU()

	// Create format map
	p.EventAttrsByFormatID = newEventAttrMap()

	p.groupFds = make([]int, ncpu)

	for cpu := 0; cpu < ncpu; cpu++ {
		p.groupFds[cpu] = int(-1)

		for _, ea := range eas {
			flags := p.options.flags | PERF_FLAG_FD_CLOEXEC

			// Fixup
			ea.Size = sizeofPerfEventAttrVer5
			ea.SampleType |= PERF_SAMPLE_CPU | PERF_SAMPLE_STREAM_ID | PERF_SAMPLE_IDENTIFIER | PERF_SAMPLE_TIME
			fd, err := open(ea, -1, cpu, p.groupFds[cpu], flags)
			if err != nil {
				glog.Fatal(err)
			}

			streamID, err := unix.IoctlGetInt(fd, PERF_EVENT_IOC_ID)
			if err != nil {
				glog.Fatal(err)
			}

			p.EventAttrsByFormatID[uint64(streamID)] = ea

			p.fds = append(p.fds, fd)

			if p.groupFds[cpu] < 0 {
				// NB: We must open ring buffer before we can redirect output to it
				rb, err := newRingBuffer(fd, p.options.ringBufferNumPages)
				if err != nil {
					glog.Fatal(err)
				}

				p.ringBuffers = append(p.ringBuffers, rb)

				p.groupFds[cpu] = fd
			}
		}

		if p.eventAttrs == nil {
			p.eventAttrs = eas
		}
	}

	return nil
}

func (p *Perf) Run(onSample func(Sample)) error {
	p.onSample = onSample

	pollFds := make([]unix.PollFd, len(p.groupFds))

	// Enable all events
	for i, fd := range p.groupFds {
		err := enable(fd)
		if err != nil {
			glog.Fatal(err)
		}

		pollFds[i].Fd = int32(fd)
		pollFds[i].Events = unix.POLLIN
	}

	for {
		n, err := unix.Poll(pollFds, -1)
		if err != nil && err != unix.EINTR {
			return err
		}

		if n > 0 {
			for i, fd := range pollFds {
				if (fd.Revents & unix.POLLIN) != 0 {
					p.ringBuffers[i].read(p.readSample)
				}
			}
		}
	}
}

func (p *Perf) readSample(data []byte) {
	reader := bytes.NewReader(data)
	sample := &Sample{}

	err := sample.read(reader, nil, p.EventAttrsByFormatID)
	if err != nil {
		glog.Fatal(err)
	}

	if p.onSample != nil {
		p.onSample(*sample)
	}
}
