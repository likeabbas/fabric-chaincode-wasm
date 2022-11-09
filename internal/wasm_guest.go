// Copyright the Hyperledger Fabric contributors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

package internal

import (
	"context"
	"fmt"
	"github.com/wapc/wapc-go/engines/wazero"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/wapc/wapc-go"
)

// WasmGuestInvoker is the interface that wraps the InvokeWasmOperation method.
//
//counterfeiter:generate -o fakes/wapc_guest_invoker.go --fake-name WasmGuestInvoker . WasmGuestInvoker
type WasmGuestInvoker interface {
	InvokeWasmOperation(operation string, payload []byte) ([]byte, error)
}

// WasmGuest encapsulates external dependencies required to invoke operations
// in Wasm guest code. Currently this uses a pool of waPC instances.
type WasmGuest struct {
	wapcModule *wapc.Module
	wapcPool   *wapc.Pool
	wapcEngine *wapc.Engine
	context    context.Context
}

func consoleLog(msg string) {
	fmt.Println(msg)
}

// NewWasmGuest returns a new WasmGuest capable of invoking Wasm operations
func NewWasmGuest(wasmFile string, proxy *FabricProxy) (*WasmGuest, error) {
	wg := &WasmGuest{}
	ctx, _ := context.WithCancel(context.Background())
	engine := wazero.Engine()

	wasmBytes, err := ioutil.ReadFile(wasmFile)
	if err != nil {
		return nil, err
	}

	module, err := engine.New(ctx, proxy.FabricCall, wasmBytes, &wapc.ModuleConfig{
		Logger: wapc.PrintlnLogger,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})

	if err != nil {
		return nil, err
	}

	wg.wapcModule = &module

	pool, err := wapc.NewPool(context.Background(), module, 10)
	if err != nil {
		return nil, err
	}
	wg.wapcPool = pool
	wg.wapcEngine = &engine
	wg.context = ctx

	return wg, nil
}

// InvokeWasmOperation invoke a Wasm guest operation
func (wg *WasmGuest) InvokeWasmOperation(operation string, payload []byte) (result []byte, err error) {
	log.Printf("[host] Getting waPC Instance\n")
	wapcInstance, err := wg.wapcPool.Get(10 * time.Millisecond)
	if err != nil {
		log.Printf("[host] error getting waPC instance: %s\n", err)
		return nil, err
	}
	defer func() {
		log.Printf("[host] Returning waPC Instance\n")
		err = wg.wapcPool.Return(wapcInstance)

		if err != nil {
			log.Printf("[host] error returning waPC instance: %s\n", err)
		}
	}()

	ctx := context.TODO()

	log.Printf("[host] Invoking operation %s\n", operation)
	result, err = wapcInstance.Invoke(ctx, operation, payload)
	if err != nil {
		log.Printf("[host] error invoking transaction: %s\n", err)
		return nil, err
	}

	return result, nil
}

// Close closes the WasmGuest, rendering it unusable for invoking further operations
func (wg *WasmGuest) Close() {
	log.Printf("[host] Closing waPC Pool")
	wg.wapcPool.Close(context.Background())

	log.Printf("[host] Closing waPC Module")
	g := *wg.wapcModule
	g.Close(wg.context)
}
