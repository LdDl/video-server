package storage

import "strings"

type StorageType uint16

const (
	STORAGE_UNDEFINED_TYPE = iota
	STORAGE_FILESYSTEM
	STORAGE_MINIO
)

var storageTypes = map[string]StorageType{
	"filesystem": STORAGE_FILESYSTEM,
	"minio":      STORAGE_MINIO,
}

// String returns string representation of the storage type
func (iotaIdx StorageType) String() string {
	return [...]string{"undefined", "filesystem", "minio"}[iotaIdx]
}

func NewStorageTypeFrom(str string) StorageType {
	if found, ok := storageTypes[strings.ToLower(str)]; ok {
		return found
	}
	return STORAGE_UNDEFINED_TYPE
}
