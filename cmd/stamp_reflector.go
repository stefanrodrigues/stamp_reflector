package main

import (
    "encoding/binary"
    "flag"
    "log"
    "net"
    "time"
)

// Constantes do Protocolo
const (
    NTP_EPOCH_OFFSET = 2208988800 // Segundos entre a época Unix (1970) e a época NTP (1900)
)

// TwampTestPacket define a estrutura do pacote de teste TWAMP não autenticado.
// A estrutura é baseada na RFC 5357.
type TwampTestPacket struct {
    Sequence           uint32
    Timestamp          uint64
    ErrorEstimate      uint16
    MBZ                [2]byte // Deve ser zero
    ReceiveTimestamp   uint64
    SenderSequence     uint32
    SenderTimestamp    uint64
    SenderErrorEstimate uint16
    MBZ2               [2]byte // Deve ser zero
    SenderTTL          byte
    Padding            [15]byte // Preenchimento para completar o pacote
}

// toNTPTimestamp converte um objeto time.Time do Go para o formato NTP de 64 bits.
// O formato NTP consiste em 32 bits para a parte inteira (segundos) e 32 bits para a parte fracionária.
func toNTPTimestamp(t time.Time) uint64 {
    secs := uint64(t.Unix()) + NTP_EPOCH_OFFSET
    nanos := uint64(t.Nanosecond())
    fraction := (nanos << 32) / 1e9
    return (secs << 32) | fraction
}

func main() {
    // Configuração de flags para o endereço de escuta
    listenAddr := flag.String("addr", ":862", "O endereço e a porta UDP para escutar (ex: :862 ou 192.168.1.1:862)")
    flag.Parse()

    // Resolvendo o endereço UDP
    addr, err := net.ResolveUDPAddr("udp", *listenAddr)
    if err != nil {
        log.Fatalf("Erro ao resolver o endereço: %v", err)
    }

    // Iniciando o listener na porta UDP especificada
    conn, err := net.ListenUDP("udp", addr)
    if err != nil {
        log.Fatalf("Erro ao iniciar o listener UDP: %v", err)
    }
    defer conn.Close()

    log.Printf("Refletor TWAMP Light escutando em %s", addr)

    // Buffer para armazenar os pacotes recebidos
    buffer := make([]byte, 1500)

    for {
        // Lendo um pacote UDP
        n, remoteAddr, err := conn.ReadFromUDP(buffer)
        if err != nil {
           log.Printf("Erro ao ler do socket UDP: %v", err)
           continue
        }

        // Captura o timestamp de recebimento o mais rápido possível
        receiveTime := time.Now()

        // Verifica o tamanho mínimo do pacote TWAMP
        if n < 14 { // Tamanho mínimo do pacote de teste do remetente
           log.Printf("Pacote muito pequeno recebido de %s, descartando.", remoteAddr)
           continue
        }

        var requestPacket TwampTestPacket
        // Decodifica o pacote recebido
        //Sequence | uint32 | 4B
        requestPacket.Sequence = binary.BigEndian.Uint32(buffer[0:4])
        //Timestamp | uint64 | 8B
        requestPacket.Timestamp = binary.BigEndian.Uint64(buffer[4:12])
        //Error Estimate | uint16 | 2B
        requestPacket.ErrorEstimate = binary.BigEndian.Uint16(buffer[12:14])
        //TTL
        requestPacket.SenderTTL = buffer[40]

        log.Printf("Recebido pacote de teste de %s (Seq: %d)", remoteAddr, requestPacket.Sequence)

        //Raspberry
        //Send
        //00000000 ec432e2907aa318e 0000 0000 0000000000000000 00000000 0000000000000000 0000 0000 40 000000000000000000000000000000
        //Received
        //00000000 ec432e29084fa186 0000 0000 ec432e2907aa318e 00000000 ec432e29085fce87 0000 0000 ff 00

        // Prepara o pacote de resposta
        var responsePacket TwampTestPacket
        responsePacket.Sequence = requestPacket.Sequence
        responsePacket.Timestamp = toNTPTimestamp(time.Now()) // Timestamp de Envio do Reflector (T3)
        responsePacket.ErrorEstimate = requestPacket.ErrorEstimate // Pode ser ajustado se houver sincronização de relógio
        //responsePacket.MBZ = 0
        responsePacket.ReceiveTimestamp = toNTPTimestamp(receiveTime) // Timestamp de Recebimento do Reflector (T2)
        responsePacket.SenderSequence = requestPacket.Sequence
        responsePacket.SenderTimestamp = requestPacket.Timestamp // Timestamp de Recebimento do Reflector (T1)
        responsePacket.SenderErrorEstimate = 0 // Refletor não precisa estimar seu próprio erro
        //responsePacket.MBZ2 = 0
        responsePacket.SenderTTL = requestPacket.SenderTTL // Copia do pacote recebido
        //responsePacket.Padding = 0

        // Codifica o pacote de resposta para bytes --> 56Bytes

        responseBuffer := make([]byte, 56) // Tamanho padrão do pacote de resposta
        binary.BigEndian.PutUint32(responseBuffer[0:4], responsePacket.Sequence)
        binary.BigEndian.PutUint64(responseBuffer[4:12], responsePacket.Timestamp)
        binary.BigEndian.PutUint16(responseBuffer[12:14], responsePacket.ErrorEstimate)
        // MBZ (bytes 14-15) já são zero
        binary.BigEndian.PutUint64(responseBuffer[16:24], responsePacket.ReceiveTimestamp)
        binary.BigEndian.PutUint32(responseBuffer[24:28], responsePacket.SenderSequence)
        binary.BigEndian.PutUint64(responseBuffer[28:36], responsePacket.SenderTimestamp)
        binary.BigEndian.PutUint16(responseBuffer[36:38], responsePacket.SenderErrorEstimate)
        // MBZ2 (bytes 38-39) já são zero
        responseBuffer[40] = responsePacket.SenderTTL
        // Padding (bytes 41-56) já são zero

        // Envia o pacote de volta ao remetente
        _, err = conn.WriteToUDP(responseBuffer, remoteAddr)
        if err != nil {
           log.Printf("Erro ao enviar resposta para %s: %v", remoteAddr, err)
        }
    }
}
