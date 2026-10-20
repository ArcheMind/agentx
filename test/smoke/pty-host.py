import errno
import fcntl
import os
import pty
import select
import signal
import struct
import subprocess
import sys
import termios

master, slave = pty.openpty()
fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 140, 0, 0))
child = subprocess.Popen(sys.argv[1:], stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
os.close(slave)

def terminate(signum, frame):
    os.killpg(child.pid, signal.SIGTERM)

signal.signal(signal.SIGTERM, terminate)
try:
    while child.poll() is None:
        readable, _, _ = select.select([master, sys.stdin.buffer], [], [], 0.2)
        for source in readable:
            if source == master:
                try:
                    chunk = os.read(master, 65536)
                except OSError as error:
                    if error.errno == errno.EIO:
                        break
                    raise
                sys.stdout.buffer.write(chunk)
                sys.stdout.buffer.flush()
            else:
                chunk = os.read(sys.stdin.fileno(), 65536)
                if chunk:
                    os.write(master, chunk)
finally:
    if child.poll() is None:
        os.killpg(child.pid, signal.SIGTERM)
    child.wait()
    os.close(master)
