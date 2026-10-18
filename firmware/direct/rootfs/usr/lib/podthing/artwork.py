#!/usr/bin/python3
"""Read-only libgpod ArtworkDB thumbnail worker for stock iPod Classics.

This separate process contains possible errors in legacy C decoders. It loads
iTunesDB/ArtworkDB once and never invokes libgpod's write/sync operations.
stdin: unsigned track id per line, or QUIT. stdout: READY, OK <tmp>, NONE.
"""
import ctypes as C
import os
import sys
import tempfile

def load_library():
    gpod=C.CDLL("libgpod.so.4")
    pix=C.CDLL("libgdk_pixbuf-2.0.so.0")
    obj=C.CDLL("libgobject-2.0.so.0")
    gpod.itdb_parse.argtypes=[C.c_char_p,C.POINTER(C.c_void_p)]
    gpod.itdb_parse.restype=C.c_void_p
    gpod.itdb_free.argtypes=[C.c_void_p]
    gpod.itdb_free.restype=None
    gpod.itdb_track_by_id.argtypes=[C.c_void_p,C.c_uint32]
    gpod.itdb_track_by_id.restype=C.c_void_p
    gpod.itdb_track_get_thumbnail.argtypes=[C.c_void_p,C.c_int32,C.c_int32]
    gpod.itdb_track_get_thumbnail.restype=C.c_void_p
    pix.gdk_pixbuf_savev.argtypes=[C.c_void_p,C.c_char_p,C.c_char_p,C.c_void_p,C.c_void_p,C.POINTER(C.c_void_p)]
    pix.gdk_pixbuf_savev.restype=C.c_int32
    obj.g_object_unref.argtypes=[C.c_void_p]
    return gpod,pix,obj

def run(root):
    gpod,pix,obj=load_library()
    err=C.c_void_p()
    db=gpod.itdb_parse(os.fsencode(root),C.byref(err))
    if not db:
        print("FAIL ArtworkDB parse unavailable",flush=True)
        return 1
    artwork_db=os.path.join(root,"iPod_Control","Artwork","ArtworkDB")
    has_artwork_db=os.path.isfile(artwork_db)
    if not has_artwork_db:
        print("ArtworkDB not found at expected iPod path",file=sys.stderr,flush=True)
    print("READY",flush=True)
    try:
        for line in sys.stdin:
            line=line.strip()
            if line=="QUIT":break
            try:
                track_id=int(line)
                if not (0<=track_id<=0xffffffff):
                    print("NONE BAD_TRACK_ID",flush=True);continue
                track=gpod.itdb_track_by_id(db,track_id)
                if not track:
                    print("NONE TRACK_ID_NOT_IN_LIBGPOD_DB",flush=True);continue
                if not has_artwork_db:
                    print("NONE ARTWORK_DB_NOT_PRESENT",flush=True);continue
                image=gpod.itdb_track_get_thumbnail(track,200,200)
                if not image:
                    print("NONE TRACK_HAS_NO_THUMBNAIL",flush=True);continue
                try:
                    with tempfile.NamedTemporaryFile(prefix="podthing-art-",suffix=".png",delete=False) as tmp:
                        path=tmp.name
                    saved=pix.gdk_pixbuf_savev(image,os.fsencode(path),b"png",None,None,C.byref(err))
                    if saved:
                        print("OK "+path,flush=True)
                    else:
                        os.unlink(path)
                        print("NONE PNG_SAVE_FAILED",flush=True)
                finally:
                    obj.g_object_unref(image)
            except Exception as e:
                print("NONE DECODER_ERROR "+type(e).__name__,flush=True)
                print("cover decode failed:",e,file=sys.stderr,flush=True)
    finally:
        gpod.itdb_free(db)
    return 0

if __name__=="__main__":
    if len(sys.argv)!=2:
        sys.exit(2)
    try:
        sys.exit(run(sys.argv[1]))
    except Exception as e:
        print("FAIL "+str(e),flush=True)
        sys.exit(1)
