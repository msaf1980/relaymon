#! /usr/bin/env python

import logging
import sys
import os
import re
import select
import signal
import subprocess
import netifaces as ni

def checkIPS(iface, ips):
    status = []
    found = 0
    if len(ips) > 0:
        for i in range(len(ips)):
            status.append((False, None))

        for ip in ni.ifaddresses(iface)[ni.AF_INET]:
            for i in range(len(ips)):
                if ip['addr'] == ips[i][0]:
                    found += 1
                    status[i] = (True, ip['netmask'])

    return found, status


def printIPS(iface, ips, status, founded = True):
    for i in range(len(ips)):
        if status[i][0] and founded:
            sys.stderr.write("configured %s on %s\n" % (ips[i][0], iface))
        elif not status[i][0] and not founded:
            sys.stderr.write("not configured %s on %s\n" % (ips[i][0], iface))

def validateIPS(iface, ips, status):
    valid = True
    for i in range(len(ips)):
        if status[i][0]:
            if status[i][1] != ips[i][1]:
                sys.stderr.write("configured %s with different netmask %s on %s\n" % (ips[i][0], status[i][1], iface))
                valid = False
        else:
            sys.stderr.write("not configured %s on %s\n" % (ips[i][0], iface))
            valid = False

    return valid

# Base test function
def test(name, config, service, iface, ips, output, stageAction, stageResult):
    test = "test Fail Success Fail #1"
    success = False
    try:
        proc = subprocess.Popen([name, "-config", config], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        ec = proc.poll()
        if ec is not None:
            sys.stderr.write("%s exit with code %s on start\n" % (name, ec))
            sys.exit(ec)

        success = True

        stage = 0
        step = 0
        if stageAction[0] is not None:
            sys.stderr.write("stageAction not implemented\n")
            return False

        while stage < len(output):
            s = select.select([proc.stdout, proc.stderr], [], [], 60)
            if len(s[0]) == 0:
                sys.stderr.write("read from stdout timeout\n")
                success = False
                break
            if proc.stderr in s[0]:
                sys.stderr.write("stderr non empthy\n")
                success = False
                break
            if proc.stdout in s[0]:
                line = proc.stdout.readline()
                sys.stdout.write(line)
                match = re.match(output[stage][step][0], line)
                if match is not None:
                    groups = match.groups()
                    i = 1
                    for g in groups:
                        if g != output[stage][step][i]:
                            sys.stderr.write("stdout mismatched at %d (got %s, want %s): %s" % (i, g, output[stage][step][i], line))
                            success = False
                            break
                        i += 1
                    step += 1
                    sys.stdout.write("step %s ended\n" % step)
                    if step == len(output[stage]):
                        # Step ended
                        found, status = checkIPS(iface, ips)
                        if stageResult[stage]:
                            if not validateIPS(iface, ips, status):
                                sys.stdout.write("stage %s failed\n" % stage)
                                success = False
                                break
                        elif found > 0:
                            printIPS(iface, ips, status)
                            sys.stdout.write("stage %s failed\n" % stage)
                            success = False
                            break

                        sys.stdout.write("stage %s ended\n" % stage)
                        stage += 1

        sys.stdout.write("test shutting down\n")
        proc.terminate()
        ec = proc.wait()
        for line in proc.stdout:
            sys.stdout.write(line)
        for line in proc.stderr:
            sys.stderr.write(line)
        if ec == 0 and success:
            sys.stdout.write("SUCCESS %s\n" % test)
            return True
        else:
            sys.stderr.write("%s exit with code %s\n" % (name, ec))
            sys.stdout.write("FAILED %s\n" % test)
            return False
    except Exception as e:
        sys.stderr.write(str(e))
        return False


def testFailSuccessFail1(name, config, service, iface, ips):
    output = []
    stageAction = []
    stageResult = []

    # Step 0 (startup). Go to failed state
    stageAction.append(None)
    stageResult.append(False)
    output.append([
        (
            '{"level":"([a-z]+)","service":"%s","time":"[0-9-T:Z\+-]+","message":"state changed to ([a-z]+)"}' % service,
            "info", "success", None
        ),
        (
            '{"level":"([a-z]+)","service":"carbon-c-relay clusters","time":"[0-9-T:Z\+-]+","message":"state changed to ([a-z]+)"}',
            "error", "error", None
        ),
        (
            '{"level":"([a-z]+)","action":"([a-z]+)","time":"[0-9-T:Z\+-]+","message":"go to ([a-z]+) state"}',
            "error", "down", "error"
        ),
        (
            '{"level":"([a-z]+)","action":"([a-z]+)","time":"[0-9-T:Z\+-]+","message":"([A-Z]+)"}',
            "error", "down", "DOWN"
        ),
    ])


    # (
    #     '{"level":"([a-z]+)","time":"2020-08-26T10:49:59Z","message":"shutdown"}', 
    # )

    return test(name, config, service, iface, ips, output, stageAction, stageResult)

def main():
    if os.getuid() != 0:
        sys.stderr.write("run as root for network configuration testing\n")
        sys.exit(255)

    name = "./relaymon"
    service = "sshd"
    config = "relaymon-test.yml"
    iface = "lo"
    ips = [("192.168.155.10", "24"), ("192.168.155.11", "24")]

    sys.stderr.write("%s will be stopped/restarted during test\n" % service)

    success = True

    found, status = checkIPS(iface, ips)
    if found > 0:
        printIPS(iface, ips, status)
        sys.exit(255)

    if not testFailSuccessFail1(name, config, service, iface, ips):
        success = False

    if success:
        sys.stdout.write("SUCCESS\n")
        sys.exit(0)
    else:
        sys.stdout.write("FAILED\n")
        sys.exit(1)

    
if __name__ == "__main__":
    main()
