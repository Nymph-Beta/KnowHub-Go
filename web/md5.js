const SHIFT_AMOUNTS = [
  7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22,
  5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20,
  4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23,
  6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21,
];

const TABLE = Array.from({ length: 64 }, (_, index) => Math.floor(Math.abs(Math.sin(index + 1)) * 2 ** 32) >>> 0);

function leftRotate(value, amount) {
  return ((value << amount) | (value >>> (32 - amount))) >>> 0;
}

function toHexLE(value) {
  const b0 = value & 0xff;
  const b1 = (value >>> 8) & 0xff;
  const b2 = (value >>> 16) & 0xff;
  const b3 = (value >>> 24) & 0xff;
  return [b0, b1, b2, b3].map((part) => part.toString(16).padStart(2, "0")).join("");
}

export function md5ArrayBuffer(buffer) {
  const input = new Uint8Array(buffer);
  const originalLength = input.length;
  const bitLength = originalLength * 8;
  const paddedLength = (((originalLength + 8) >>> 6) + 1) * 64;
  const padded = new Uint8Array(paddedLength);

  padded.set(input);
  padded[originalLength] = 0x80;

  let lowBits = bitLength >>> 0;
  let highBits = Math.floor(bitLength / 2 ** 32) >>> 0;
  for (let i = 0; i < 4; i += 1) {
    padded[paddedLength - 8 + i] = lowBits & 0xff;
    lowBits >>>= 8;
  }
  for (let i = 0; i < 4; i += 1) {
    padded[paddedLength - 4 + i] = highBits & 0xff;
    highBits >>>= 8;
  }

  let a0 = 0x67452301;
  let b0 = 0xefcdab89;
  let c0 = 0x98badcfe;
  let d0 = 0x10325476;

  for (let offset = 0; offset < padded.length; offset += 64) {
    const words = new Uint32Array(16);
    for (let index = 0; index < 16; index += 1) {
      const wordOffset = offset + index * 4;
      words[index] =
        padded[wordOffset] |
        (padded[wordOffset + 1] << 8) |
        (padded[wordOffset + 2] << 16) |
        (padded[wordOffset + 3] << 24);
    }

    let a = a0;
    let b = b0;
    let c = c0;
    let d = d0;

    for (let index = 0; index < 64; index += 1) {
      let f;
      let g;

      if (index < 16) {
        f = (b & c) | (~b & d);
        g = index;
      } else if (index < 32) {
        f = (d & b) | (~d & c);
        g = (5 * index + 1) % 16;
      } else if (index < 48) {
        f = b ^ c ^ d;
        g = (3 * index + 5) % 16;
      } else {
        f = c ^ (b | ~d);
        g = (7 * index) % 16;
      }

      const temp = d;
      d = c;
      c = b;
      const sum = (a + f + TABLE[index] + words[g]) >>> 0;
      b = (b + leftRotate(sum, SHIFT_AMOUNTS[index])) >>> 0;
      a = temp;
    }

    a0 = (a0 + a) >>> 0;
    b0 = (b0 + b) >>> 0;
    c0 = (c0 + c) >>> 0;
    d0 = (d0 + d) >>> 0;
  }

  return `${toHexLE(a0)}${toHexLE(b0)}${toHexLE(c0)}${toHexLE(d0)}`;
}
