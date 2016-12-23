/*
 * Minio Cloud Storage, (C) 2016 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package dsync_test

import (
	"net/rpc"
	"sync"

	. "github.com/minio/dsync"
)

// ReconnectRPCClient is a wrapper type for rpc.Client which provides reconnect on first failure.
type ReconnectRPCClient struct {
	mutex   sync.Mutex
	rpc     *rpc.Client
	node    string
	rpcPath string
}

// newClient constructs a ReconnectRPCClient object with node and rpcPath initialized.
// It _doesn't_ connect to the remote endpoint. See Call method to see when the
// connect happens.
func newClient(node, rpcPath string) NetLocker {
	return &ReconnectRPCClient{
		node:    node,
		rpcPath: rpcPath,
	}
}

// Close closes the underlying socket file descriptor.
func (rpcClient *ReconnectRPCClient) Close() error {
	rpcClient.mutex.Lock()
	defer rpcClient.mutex.Unlock()
	// If rpc client has not connected yet there is nothing to close.
	if rpcClient.rpc == nil {
		return nil
	}
	// Reset rpcClient.rpc to allow for subsequent calls to use a new
	// (socket) connection.
	clnt := rpcClient.rpc
	rpcClient.rpc = nil
	return clnt.Close()
}

// Call makes a RPC call to the remote endpoint using the default codec, namely encoding/gob.
func (rpcClient *ReconnectRPCClient) Call(serviceMethod string, args interface{}, reply interface{}) error {
	rpcClient.mutex.Lock()
	defer rpcClient.mutex.Unlock()
	// If the rpc.Client is nil, we attempt to (re)connect with the remote endpoint.
	if rpcClient.rpc == nil {
		clnt, err := rpc.DialHTTPPath("tcp", rpcClient.node, rpcClient.rpcPath)
		if err != nil {
			return err
		}
		rpcClient.rpc = clnt
	}

	// If the RPC fails due to a network-related error, then we reset
	// rpc.Client for a subsequent reconnect.
	err := rpcClient.rpc.Call(serviceMethod, args, reply)
	if IsRPCError(err) {
		rpcClient.rpc = nil
	}
	return err
}

func (rpcClient *ReconnectRPCClient) RLock(args LockArgs) (status bool, err error) {
	err = rpcClient.Call("Dsync.RLock", &args, &status)
	return status, err
}

func (rpcClient *ReconnectRPCClient) Lock(args LockArgs) (status bool, err error) {
	err = rpcClient.Call("Dsync.Lock", &args, &status)
	return status, err
}

func (rpcClient *ReconnectRPCClient) RUnlock(args LockArgs) (status bool, err error) {
	err = rpcClient.Call("Dsync.RUnlock", &args, &status)
	return status, err
}

func (rpcClient *ReconnectRPCClient) Unlock(args LockArgs) (status bool, err error) {
	err = rpcClient.Call("Dsync.Unlock", &args, &status)
	return status, err
}

func (rpcClient *ReconnectRPCClient) ForceUnlock(args LockArgs) (status bool, err error) {
	err = rpcClient.Call("Dsync.ForceUnlock", &args, &status)
	return status, err
}

func (rpcClient *ReconnectRPCClient) ServerAddr() string {
	return rpcClient.node
}

func (rpcClient *ReconnectRPCClient) ServiceEndpoint() string {
	return rpcClient.rpcPath
}

// IsRPCError returns true if the error value is due to a network related
// failure, false otherwise.
func IsRPCError(err error) bool {
	if err == nil {
		return false
	}
	// The following are net/rpc specific errors that indicate that
	// the connection may have been reset. Reset rpcClient.rpc to nil
	// to trigger a reconnect in future.
	if err == rpc.ErrShutdown {
		return true
	}
	return false
}
