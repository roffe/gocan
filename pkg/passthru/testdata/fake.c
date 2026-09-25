// A stub J2534 library with the argument and struct layouts the real ones
// use. Every function checks what it was handed and answers with a J2534
// error code when the binding got it wrong, so the Go side can assert on
// plain return values. Compiled by the passthru and adapters/j2534 tests.
#include <stdint.h>
#include <string.h>
typedef struct { uint32_t ProtocolID, RxStatus, TxFlags, Timestamp, DataSize, ExtraDataIndex; uint8_t Data[4128]; } PASSTHRU_MSG;
typedef struct { uint32_t Parameter, Value; } SCONFIG;
typedef struct { uint32_t NumOfParams; SCONFIG *ConfigPtr; } SCONFIG_LIST;

uint32_t PassThruOpen(const char *name, uint32_t *id) {
	if (name == 0) { *id = 7; return 0; }
	if (strcmp(name, "dev") == 0) { *id = 8; return 0; }
	return 0x1A;
}
uint32_t PassThruClose(uint32_t id) { return id == 7 ? 0 : 0x1A; }
uint32_t PassThruConnect(uint32_t dev, uint32_t proto, uint32_t flags, uint32_t baud, uint32_t *ch) {
	if (dev != 7 || proto != 5 || flags != 0x900 || baud != 500000) return 0x03;
	*ch = 9; return 0;
}
uint32_t PassThruDisconnect(uint32_t ch) { return ch == 9 ? 0 : 0x02; }
uint32_t PassThruReadMsgs(uint32_t ch, PASSTHRU_MSG *msg, uint32_t *n, uint32_t timeout) {
	if (ch != 9 || *n != 1) return 0x02;
	static int k; /* alternate frame / empty: covers ERR_BUFFER_EMPTY and a polling reader */
	if (k++ & 1) { *n = 0; return 0x10; }
	msg->ProtocolID = 5; msg->RxStatus = 0x100; msg->TxFlags = 0; msg->Timestamp = 1234;
	msg->DataSize = 6; msg->ExtraDataIndex = 6;
	memcpy(msg->Data, "\x00\x00\x02\x58\xAB\xCD", 6);
	*n = 1; return 0;
}
uint32_t PassThruWriteMsgs(uint32_t ch, PASSTHRU_MSG *msg, uint32_t *n, uint32_t timeout) {
	if (ch != 9 || *n != 1 || (timeout != 0 && timeout != 25)) return 0x02;
	static int full = 2; /* a full queue refuses the first non-blocking writes, like a MongoosePro */
	if (timeout == 0 && full) { full--; *n = 0; return 0x09; }
	if (msg->ProtocolID != 5 || msg->DataSize != 5 || msg->ExtraDataIndex != 5 || msg->TxFlags != 0x100) return 0x0A;
	if (msg->Data[2] != 0x02 || msg->Data[3] != 0x58 || msg->Data[4] != 0x42) return 0x0A;
	return 0;
}
uint32_t PassThruStartMsgFilter(uint32_t ch, uint32_t type, PASSTHRU_MSG *mask, PASSTHRU_MSG *pat, PASSTHRU_MSG *fc, uint32_t *id) {
	if (ch != 9 || type != 1 || fc != 0) return 0x0A;
	if (mask->DataSize != 4 || mask->Data[2] != 0x07 || mask->Data[3] != 0xFF || pat->Data[3] != 0x40) return 0x0A;
	*id = 42; return 0;
}
uint32_t PassThruIoctl(uint32_t h, uint32_t id, void *in, void *out) {
	SCONFIG_LIST *l = in;
	if (h != 9) return 0x02;
	switch (id) {
	case 1: if (l->NumOfParams != 1 || out != 0) return 0x05; l->ConfigPtr[0].Value = 99; return 0;
	case 2: if (l->NumOfParams != 2 || l->ConfigPtr[1].Parameter != 0x8001 || l->ConfigPtr[1].Value != 0x100 || out != 0) return 0x05; return 0;
	case 8: case 10: return in == 0 && out == 0 ? 0 : 0x04;
	case 5: return ((PASSTHRU_MSG *)in)->DataSize == 3 && out != 0 ? 0x07 : 0x0A;
	}
	return 0x0F;
}
uint32_t PassThruReadVersion(uint32_t dev, char *fw, char *dll, char *api) {
	if (dev != 7) return 0x1A;
	strcpy(fw, "1.0"); strcpy(dll, "fake"); strcpy(api, "04.04"); return 0;
}
uint32_t PassThruGetLastError(char *desc) { strcpy(desc, "boom"); return 0; }
