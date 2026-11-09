#!/usr/bin/env python3
"""Render a project namespace policy for review; never modifies a cluster."""
import argparse
from pathlib import Path
import re

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--project-id', required=True)
p.add_argument('--namespace', required=True)
p.add_argument('--output', type=Path, required=True)
a = p.parse_args()
if not re.fullmatch(r'[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}', a.project_id):
    p.error('project ID must fit a Kubernetes label value')
if not re.fullmatch(r'[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?', a.namespace):
    p.error('invalid project namespace')
source = Path(__file__).with_name('project-template.yaml').read_text()
source = source.replace('__PROJECT_ID__', a.project_id).replace('__PROJECT_NAMESPACE__', a.namespace)
a.output.write_text(source)
print('Rendered project policies for review:', a.output)
