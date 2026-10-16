package memory

import (
	"testing"

	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/store"
	"github.com/berat-kaan-akcan/WarrantyWallet/backend/internal/store/storetest"
)

func TestContract(t *testing.T) {
	storetest.Run(t, func(*testing.T) store.Store { return New() })
}
