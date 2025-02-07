/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package adaptivesvc

import (
	"context"
	"strconv"
)

import (
	perrors "github.com/pkg/errors"
)

import (
	"dubbo.apache.org/dubbo-go/v3/cluster/cluster/base"
	"dubbo.apache.org/dubbo-go/v3/cluster/directory"
	"dubbo.apache.org/dubbo-go/v3/cluster/metrics"
	"dubbo.apache.org/dubbo-go/v3/common/constant"
	"dubbo.apache.org/dubbo-go/v3/common/extension"
	"dubbo.apache.org/dubbo-go/v3/common/logger"
	"dubbo.apache.org/dubbo-go/v3/protocol"
)

type adaptiveServiceClusterInvoker struct {
	base.BaseClusterInvoker
}

func newAdaptiveServiceClusterInvoker(directory directory.Directory) protocol.Invoker {
	return &adaptiveServiceClusterInvoker{
		BaseClusterInvoker: base.NewBaseClusterInvoker(directory),
	}
}

func (ivk *adaptiveServiceClusterInvoker) Invoke(ctx context.Context, invocation protocol.Invocation) protocol.Result {
	invokers := ivk.Directory.List(invocation)
	if err := ivk.CheckInvokers(invokers, invocation); err != nil {
		return protocol.NewRPCResult(nil, err)
	}

	// get loadBalance
	lbKey := invokers[0].GetURL().GetParam(constant.LoadbalanceKey, constant.LoadBalanceKeyP2C)
	if lbKey != constant.LoadBalanceKeyP2C {
		return protocol.NewRPCResult(nil, perrors.Errorf("adaptive service not supports %s load balance", lbKey))
	}
	lb := extension.GetLoadbalance(lbKey)

	// select a node by the loadBalance
	invoker := lb.Select(invokers, invocation)

	// invoke
	result := invoker.Invoke(ctx, invocation)

	// update metrics
	remainingStr := result.Attachment(constant.AdaptiveServiceRemainingKey, "").(string)
	remaining, err := strconv.Atoi(remainingStr)
	if err != nil {
		logger.Warnf("the remaining is unexpected, we need a int type, but we got %s, err: %v.", remainingStr, err)
		return result
	}
	logger.Debugf("[adasvc cluster] The server status was received successfully, %s: %#v",
		constant.AdaptiveServiceRemainingKey, remainingStr)
	err = metrics.LocalMetrics.SetMethodMetrics(invoker.GetURL(),
		invocation.MethodName(), metrics.HillClimbing, uint64(remaining))
	if err != nil {
		logger.Warnf("adaptive service metrics update is failed, err: %v", err)
		return protocol.NewRPCResult(nil, err)
	}

	return result
}
