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

package functional

import (
	"syscall"
	"testing"

	api "github.com/capsule8/capsule8/api/v0"
	"github.com/capsule8/capsule8/pkg/expression"
	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes/wrappers"
)

//
// The process inside the container sets alarms for 87, 88, and 89
// seconds. This test applies appropriate filters and ensures that we
// only see events for 88 and 89, not 87.
//
const (
	alarmSeconds1 = 88
	alarmSeconds2 = 89
)

type syscallTest struct {
	testContainer *Container
	pid           string

	sawAlarm1Enter bool
	sawAlarm1Exit  bool
	sawAlarm2Enter bool
	sawAlarm2Exit  bool
}

func (st *syscallTest) BuildContainer(t *testing.T) string {
	c := NewContainer(t, "syscall")
	err := c.Build()
	if err != nil {
		t.Error(err)
		return ""
	}

	glog.V(2).Infof("Built container %s\n", c.ImageID[0:12])
	st.testContainer = c

	return st.testContainer.ImageID
}

func (st *syscallTest) RunContainer(t *testing.T) {
	err := st.testContainer.Run()
	if err != nil {
		t.Error(err)
	}
	glog.V(2).Infof("Running container %s\n", st.testContainer.ImageID[0:12])
}

func (st *syscallTest) CreateSubscription(t *testing.T) *api.Subscription {
	idExpr := expression.Equal(
		expression.Identifier("id"),
		expression.Value(uint64(syscall.SYS_ALARM)))

	enterExpr := expression.LogicalAnd(idExpr,
		expression.Equal(
			expression.Identifier("arg0"),
			expression.Value(uint64(alarmSeconds2))))

	exitExpr := expression.LogicalAnd(idExpr,
		expression.Equal(
			expression.Identifier("ret"),
			expression.Value(uint64(alarmSeconds2))))

	syscallEvents := []*api.SyscallEventFilter{
		&api.SyscallEventFilter{
			Type: api.SyscallEventType_SYSCALL_EVENT_TYPE_ENTER,
			Id:   &wrappers.Int64Value{Value: syscall.SYS_ALARM},
			Arg0: &wrappers.UInt64Value{Value: alarmSeconds1},
		},
		&api.SyscallEventFilter{
			Type: api.SyscallEventType_SYSCALL_EVENT_TYPE_EXIT,
			Id:   &wrappers.Int64Value{Value: syscall.SYS_ALARM},
			Ret:  &wrappers.Int64Value{Value: alarmSeconds1},
		},
		&api.SyscallEventFilter{
			Type:             api.SyscallEventType_SYSCALL_EVENT_TYPE_ENTER,
			FilterExpression: enterExpr,
		},
		&api.SyscallEventFilter{
			Type:             api.SyscallEventType_SYSCALL_EVENT_TYPE_EXIT,
			FilterExpression: exitExpr,
		},
	}

	eventFilter := &api.EventFilter{
		SyscallEvents: syscallEvents,
	}

	return &api.Subscription{
		EventFilter: eventFilter,
	}
}

func (st *syscallTest) HandleTelemetryEvent(t *testing.T, te *api.TelemetryEvent) bool {
	glog.V(2).Infof("Got Event %+v\n", te.Event)
	switch event := te.Event.Event.(type) {
	case *api.Event_Container:
		switch event.Container.Type {
		case api.ContainerEventType_CONTAINER_EVENT_TYPE_CREATED:
			return true

		default:
			t.Errorf("Unexpected Container event %+v\n", event)
			return false
		}

	case *api.Event_Syscall:
		if event.Syscall.Id != syscall.SYS_ALARM {
			t.Errorf("Expected syscall number %d, got %d\n",
				syscall.SYS_ALARM, event.Syscall.Id)
		}

		switch event.Syscall.Type {
		case api.SyscallEventType_SYSCALL_EVENT_TYPE_ENTER:
			if event.Syscall.Arg0 == alarmSeconds1 {
				st.sawAlarm1Enter = true
			} else if event.Syscall.Arg0 == alarmSeconds2 {
				st.sawAlarm2Enter = true
			} else {
				t.Errorf("Unexpected alarm arg0 %d\n",
					event.Syscall.Arg0)

				return false
			}

		case api.SyscallEventType_SYSCALL_EVENT_TYPE_EXIT:
			if event.Syscall.Ret == alarmSeconds1 {
				st.sawAlarm1Exit = true
			} else if event.Syscall.Ret == alarmSeconds2 {
				st.sawAlarm2Exit = true
			} else {
				t.Errorf("Unexpected syscall return %d\n",
					event.Syscall.Ret)

				return false
			}

		}

	default:
		t.Errorf("Unexpected event type %T\n", event)
		return false
	}

	return !(st.sawAlarm1Enter && st.sawAlarm1Exit && st.sawAlarm2Enter && st.sawAlarm2Exit)
}

//
// TestSyscall is a functional test for SyscallEventFilter subscriptions.
// It exercises filtering on Arg0 for SYSCALL_EVENT_TYPE_ENTER events, and
// filtering on Ret for SYSCALL_EVENT_TYPE_EXIT events.
//
func TestSyscall(t *testing.T) {
	// t.Skip("Skipping syscall test until expression engine is complete.")
	st := &syscallTest{}

	tt := NewTelemetryTester(st)
	tt.RunTest(t)
}
