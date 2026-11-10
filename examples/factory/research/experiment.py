"""CPU-only reproducible regression fixture; Python standard library only."""
import argparse
import hashlib
import json
import math
from pathlib import Path
import random
import time


def experiment(seed=42, regularisation=0.0):
    if not math.isfinite(regularisation) or regularisation < 0:
        raise ValueError('regularisation must be finite and nonnegative')
    rng = random.Random(seed)
    data = [(rng.uniform(-1, 1), rng.gauss(0, 0.1)) for _ in range(256)]
    train, test = data[:192], data[192:]
    started = time.monotonic()
    slope = sum(x * (2 * x + noise) for x, noise in train) / (sum(x*x for x, _ in train) + regularisation)
    mse = sum((slope*x - (2*x + noise))**2 for x, noise in test) / len(test)
    return {'schema_version': 1, 'seed': seed, 'regularisation': regularisation,
            'dataset_sha256': hashlib.sha256(json.dumps(data, separators=(',', ':')).encode()).hexdigest(),
            'train_samples': len(train), 'test_samples': len(test), 'slope': slope,
            'held_out_mse': mse, 'elapsed_seconds': time.monotonic()-started}


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--seed', type=int, default=42)
    parser.add_argument('--regularisation', type=float, default=0)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    result = experiment(args.seed, args.regularisation)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + '\n')
