package vorbis

/*
#include "stb/stb_vorbis.c"

static short short_index(short *s, int i) {
  return s[i];
}
static float float_index(float **f, int i, int j) {
  return f[i][j];
}
 */
import "C"

import (
   "bytes"
   "errors"
   "fmt"
   "io"
   "time"
   "unsafe"
   "log"
)

// Decode decodes b as an ogg-vorbis file, returning the interleaved channel data and
// number of channels if the data was not ogg-vorbis.
func Decode(b []byte) (data []int16, channels int, sampleRate int, err error) {
   if len(b) == 0 {
      return
   }

   raw := (*C.uchar)(unsafe.Pointer(&b[0]))
   var Cchannels C.int
   var Crate C.int
   var output *C.short
   var samples = C.stb_vorbis_decode_memory(raw, C.int(len(b)), &Cchannels, &Crate, &output)

   log.Println("ogg decode samples:", samples, " buffer size:", len(b), "bitd:", len(b)/int(samples))

   if samples < 0 {
      err = errors.New("Failed to decode vorbis")
      return
   }
   defer C.free(unsafe.Pointer(output))

   data = make([]int16, int(samples)*int(Cchannels))
   for i := range data {
      data[i] = int16(C.short_index(output, C.int(i)))
   }

   return data, int(Cchannels), int(Crate), nil
}

// Length returns the duration of an ogg-vorbis file.
func Length(b []byte) (time.Duration, error) {
   raw := (*C.uchar)(unsafe.Pointer(&b[0]))
   var cerror C.int
   v := C.stb_vorbis_open_memory(raw, C.int(len(b)), &cerror, nil)
   if cerror != 0 {
      return 0, fmt.Errorf("vorbis: stb_vorbis_open_memory: %v", cerror)
   }
   secs := C.stb_vorbis_stream_length_in_seconds(v)
   C.stb_vorbis_close(v)
   dur := time.Duration(secs) * time.Second
   return dur, nil
}

// New opens the Vorbis file from r, which is then prepared for playback.
func New(r io.Reader) (*Vorbis, error) {
   v := &Vorbis{
      r: r,
   }
   b := make([]byte, 2048)
   var datablock_memory_consumed_in_bytes C.int
   var cerror C.int
   for {
      if _, err := v.read(b); err != nil {
         return nil, err
      }
      datablock := (*C.uchar)(unsafe.Pointer(&b[0]))
      sb := C.stb_vorbis_open_pushdata(
         datablock,
         C.int(len(b)),
         &datablock_memory_consumed_in_bytes,
         &cerror,
         nil,
      )
      v.prepend(b[datablock_memory_consumed_in_bytes:])
      if cerror == C.VORBIS_need_more_data {
         b = make([]byte, len(b)*2)
         continue
      } else if cerror != 0 {
         return nil, fmt.Errorf("vorbis: stb_vorbis_open_pushdata: %v", cerror)
      }
      v.v = sb
      break
   }
   info := C.stb_vorbis_get_info(v.v)
   v.Channels = int(info.channels)
   v.SampleRate = int(info.sample_rate)
   return v, v.err()
}

func (v *Vorbis) err() error {
   if cerror := C.stb_vorbis_get_error(v.v); cerror != 0 {
      return fmt.Errorf("vorbis error: %v", cerror)
   }
   return nil
}

// Vorbis is an Ogg Vorbis decoder.
type Vorbis struct {
   Channels   int
   SampleRate int
   buf        []byte
   r          io.Reader
   v          *C.stb_vorbis
}

func (v *Vorbis) read(p []byte) (int, error) {
   m := io.MultiReader(bytes.NewReader(v.buf), v.r)
   n, err := io.ReadFull(m, p)
   if n < len(v.buf) {
      v.buf = v.buf[n:]
   } else {
      v.buf = nil
   }
   return n, err
}

func (v *Vorbis) prepend(p []byte) {
   b := make([]byte, len(p)+len(v.buf))
   copy(b, p)
   copy(b[len(p):], v.buf)
   v.buf = b
}

// Decode decodes the next frame of data. Samples are returned in
// channel-interleaved order.
// TODO 这里的实现由bug！！！
func (v *Vorbis) Decode() (data []float32, err error) {
   b := make([]byte, 2048)
   var channels, samples C.int
   var output **C.float
   for {
      if _, err := v.read(b); err != nil {
         return nil, err
      }
      datablock := (*C.uchar)(unsafe.Pointer(&b[0]))
      used := C.stb_vorbis_decode_frame_pushdata(
         v.v,
         datablock,
         C.int(len(b)),
         &channels,
         &output,
         &samples,
      )
      v.prepend(b[used:])
      if used == 0 {
         b = make([]byte, len(b)*2)
         continue
      }
      break
   }
   chans := int(channels)
   samp := int(samples)
   data = make([]float32, chans*samp)
   var n int
   for s := 0; s < samp; s++ {
      for c := 0; c < chans; c++ {
         data[n] = float32(C.float_index(output, C.int(c), C.int(s)))
         n++
      }
   }
   if err == nil {
      err = v.err()
   }
   return
}

// Close closes the vorbis file and frees its used memory.
func (v *Vorbis) Close() {
   if v.v != nil {
      C.stb_vorbis_close(v.v)
   }
   v.v = nil
}