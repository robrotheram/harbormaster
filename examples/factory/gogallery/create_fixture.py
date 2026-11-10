"""Create synthetic PNG inputs and a local-only GoGallery configuration."""
import argparse
import json
from pathlib import Path
import struct
import zlib


def png(width, height):
    def chunk(kind, data):
        return struct.pack('>I', len(data)) + kind + data + struct.pack('>I', zlib.crc32(kind + data))
    pixels = b''.join(b'\0' + bytes((64, 128, y % 256)) * width for y in range(height))
    return (b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('>IIBBBBB', width, height, 8, 2, 0, 0, 0))
            + chunk(b'IDAT', zlib.compress(pixels)) + chunk(b'IEND', b''))


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('directory', type=Path)
    root = parser.parse_args().directory.resolve()
    # Refuse reuse so an experiment cannot silently mutate an earlier fixture.
    root.mkdir(parents=True, exist_ok=False)
    for name, width, height in [('album/landscape.png', 96, 64), ('album/nested/portrait.png', 64, 96), ('square.png', 80, 80)]:
        path = root / 'photos' / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(png(width, height))
    (root / 'gallery.yaml').write_text('''ui:
  aienabled: false
gallery:
  name: Synthetic gallery
  basepath: BASEPATH
  destpath: DESTPATH
  url: http://localhost:8080
  theme: EmeraldNoir
  imagesPerPage: 50
  useoriginal: false
'''.replace('BASEPATH', json.dumps(str(root / 'photos'))).replace('DESTPATH', json.dumps(str(root / 'site'))))
