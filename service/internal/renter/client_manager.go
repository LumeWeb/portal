package renter

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.etcd.io/etcd/client/v3"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

type ClientType string

const (
	ClientTypeBus    ClientType = "bus"
	ClientTypeWorker ClientType = "worker"
	ClusterKey       string     = "/discovery/renterd"
)

const (
	nodeTTL = 30 * time.Second // Time after which a node is considered dead if not updated
)

type NodeInfo struct {
	URL       string     `json:"url"`
	Type      ClientType `json:"type"`
	LastSeen  time.Time  `json:"last_seen"`
	Priority  int        `json:"priority"`
	IsHealthy bool       `json:"is_healthy"`
}

type ClientManager struct {
	ctx       core.Context
	etcdKey   string
	nodes     map[ClientType][]NodeInfo
	nodesLock sync.RWMutex
	logger    *core.Logger
}

func NewClientManager(ctx core.Context) *ClientManager {
	/*	etcdKey := ""

		if ctx.GetConfig().GetConfig().Core.ClusterEnabled() && ctx.GetConfig().GetConfig().Core.Clustered.EtcdEnabled() {
			etcdKey = ctx.GetConfig().GetConfig().Core.Clustered.Etcd.ComputePrefix(ClusterKey)
		}*/

	return &ClientManager{
		ctx: ctx,
		//	etcdKey: etcdKey,
		nodes:  make(map[ClientType][]NodeInfo),
		logger: ctx.Logger(),
	}
}

func (cm *ClientManager) Start() error {
	/*	if !cm.ctx.GetConfig().GetConfig().Core.ClusterEnabled() {
			return nil
		}

		etcdMgr, err := cm.ctx.GetConfig().GetConfig().Core.Clustered.Etcd.GetManager(cm.logger.Logger)
		if err != nil {
			return fmt.Errorf("failed to create etcd manager: %w", err)
		}

		client := etcdMgr.Client()

		// Initial load of nodes
		if err := cm.loadNodes(client); err != nil {
			return err
		}

		// Watch for changes
		go cm.watchNodes(client)
	*/
	return nil
}

func (cm *ClientManager) loadNodes(ctx context.Context, client *clientv3.Client) error {
	ctx, span := core.TraceMethod(ctx, "ClientManager.loadNodes")
	defer span.End()

	resp, err := client.Get(ctx, cm.etcdKey, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("failed to get nodes from etcd: %w", err)
	}

	cm.nodesLock.Lock()
	defer cm.nodesLock.Unlock()

	for _, kv := range resp.Kvs {
		var node NodeInfo
		if err := json.Unmarshal(kv.Value, &node); err != nil {
			cm.logger.Error("failed to unmarshal node info", zap.Error(err))
			continue
		}
		cm.nodes[node.Type] = append(cm.nodes[node.Type], node)
	}

	return nil
}

func (cm *ClientManager) watchNodes(ctx context.Context, client *clientv3.Client) {
	watchChan := client.Watch(ctx, cm.etcdKey, clientv3.WithPrefix())
	for watchResp := range watchChan {
		for _, event := range watchResp.Events {
			_, span := core.TraceMethod(ctx, "ClientManager.watchNodes.event")
			defer span.End()

			var node NodeInfo
			if err := json.Unmarshal(event.Kv.Value, &node); err != nil {
				cm.logger.Error("failed to unmarshal node info from watch event", zap.Error(err))
				continue
			}

			cm.nodesLock.Lock()
			switch event.Type {
			case clientv3.EventTypePut:
				cm.updateNode(node)
			case clientv3.EventTypeDelete:
				cm.removeNode(node)
			}
			cm.nodesLock.Unlock()
		}
	}
}

func (cm *ClientManager) updateNode(node NodeInfo) {
	nodes := cm.nodes[node.Type]
	node.LastSeen = time.Now()
	for i, existing := range nodes {
		if existing.URL == node.URL {
			nodes[i] = node
			return
		}
	}
	cm.nodes[node.Type] = append(cm.nodes[node.Type], node)
}

func (cm *ClientManager) removeNode(node NodeInfo) {
	nodes := cm.nodes[node.Type]
	for i, existing := range nodes {
		if existing.URL == node.URL {
			cm.nodes[node.Type] = append(nodes[:i], nodes[i+1:]...)
			return
		}
	}
}

func (cm *ClientManager) GetNextNode(clientType ClientType) (*NodeInfo, error) {
	cm.nodesLock.Lock()
	nodes := cm.nodes[clientType]
	if len(nodes) == 0 {
		cm.nodesLock.Unlock()
		return nil, fmt.Errorf("no available %s nodes", clientType)
	}

	now := time.Now()
	var selectedNode *NodeInfo

	// Just find a healthy node - no need to track LastUsed locally
	for i := range nodes {
		if now.Sub(nodes[i].LastSeen) > nodeTTL {
			nodes[i].IsHealthy = false
			continue
		}

		if nodes[i].IsHealthy {
			selectedNode = &nodes[i]
			break
		}
	}
	cm.nodesLock.Unlock()

	if selectedNode == nil {
		return nil, fmt.Errorf("no healthy %s nodes available", clientType)
	}

	return selectedNode, nil
}
