package cim

func NewStatusEvent(obj string, objType ObjectType, statusType StatusType, val bool) Event {
	return Event{
		Type:   EventTypeStatusChange,
		Source: obj,
		Data: StatusEventData{
			ObjType:    objType,
			StatusType: statusType,
			Success:    val,
		},
	}
}

type StatusEventData struct {
	ObjType    ObjectType
	StatusType StatusType
	Success    bool
}

type ObjectType string

const (
	ObjectTypeSourceDir ObjectType = "SourceDirectory"
	ObjectTypeImage     ObjectType = "Image"
	ObjectTypeTag       ObjectType = "Tag"
)

type StatusType string

const (
	StatusTypeResolved StatusType = "Resolved"
	StatusTypeCopied   StatusType = "Copied"
	StatusTypeDeleted  StatusType = "Deleted"
)
