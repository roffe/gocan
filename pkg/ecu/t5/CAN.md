# CAN messages

## Vanilla T5 ( not in bootloader )

### Set address for uploading bootloader ( only byte 3 & 4 useable until bootloader is running )

    Byte    0       1       2       3       4       5       6       7
    Data    A5      ADR_HH  ADR_HL  ADR_LH  ADR_LL  LENGTH  0x00    0x00

ECU will answer on 0xC with `A5 00 00 00 00 00 00 00`

##  MyBooty

### Erase ECU

    Byte    0       1       2       3       4       5       6       7
    Data    C0      0x00    0x00    0x00    0x00    0x00    0x00    0x00

### Send Boot Vector Address SRAM

    Byte    0       1       2       3       4       5       6       7
    Data    C1      ADR_HH  ADR_HL  ADR_LH  ADR_LL  0x00    0x00    0x00

### Read memory by address

This request returns 6 bytes from starting address

    Byte    0       1       2       3       4       5       6       7
    Data    C7      ADR_HH  ADR_HL  ADR_LH  ADR_LL  0x00    0x00    0x00

### Get Flash Checksum

    Byte    0       1       2       3       4       5       6       7
    Data    C8      0x00    0x00    0x00    0x00    0x00    0x00    0x00
