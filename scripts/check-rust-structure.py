#!/usr/bin/env python3
from pathlib import Path
ROOT = Path(__file__).resolve().parents[1]
SKIP={'.git','target','node_modules','build','.svelte-kit', 'fedora-out', '.fedora-build', 'vendor'}
errs=[]
opens={'{':'}','(':')','[':']'}
closes={v:k for k,v in opens.items()}
for p in ROOT.rglob('*.rs'):
    if any(x in p.parts for x in SKIP): continue
    s=p.read_text(errors='replace'); stack=[]; i=0; line=1; state='code'; block=0; raw_hash=None
    while i<len(s):
        c=s[i]; n=s[i+1] if i+1<len(s) else ''
        if c=='\n': line+=1
        if state=='line':
            if c=='\n': state='code'
        elif state=='block':
            if c=='/' and n=='*': block+=1; i+=1
            elif c=='*' and n=='/': block-=1; i+=1; state='code' if block==0 else 'block'
        elif state=='str':
            if c=='\\': i+=1
            elif c=='"': state='code'
        elif state=='char':
            if c=='\\': i+=1
            elif c=="'": state='code'
        elif state=='raw':
            if c=='"' and s.startswith('#'*raw_hash, i+1):
                i += raw_hash
                state='code'
        else:
            if c=='/' and n=='/': state='line'; i+=1
            elif c=='/' and n=='*': state='block'; block=1; i+=1
            elif c=='r':
                j=i+1
                while j<len(s) and s[j]=='#': j+=1
                if j<len(s) and s[j]=='"':
                    raw_hash=j-(i+1); state='raw'; i=j
                elif c in opens: stack.append((c,line))
            elif c=='"': state='str'
            elif c=="'":
                # Lifetime ('a) is not a char literal. Treat as char only when a closing quote is nearby.
                j=i+1
                if j<len(s) and (s[j]=='\\' or (j+1<len(s) and s[j+1]=="'")):
                    state='char'
            elif c in opens: stack.append((c,line))
            elif c in closes:
                if not stack or stack[-1][0]!=closes[c]:
                    errs.append(f'{p}:{line}: unmatched {c}')
                    break
                stack.pop()
        i+=1
    if stack: errs.append(f'{p}: unclosed delimiters {stack[-4:]}')
    if state in ('str','raw','block'): errs.append(f'{p}: unterminated {state}')
if errs:
    print('\n'.join(errs)); raise SystemExit(1)
print('Rust delimiters: OK')
