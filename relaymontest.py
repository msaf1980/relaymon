#! /usr/bin/env python

# netifaces and netaddr modules required

import logging
import multiprocessing
import sys
import os
import re
import tempfile
import time
import traceback
import select
import signal
import socket
import subprocess
import netifaces as ni
from netaddr import IPAddress
from string import Template


import argparse


debug = False


def parse_cmdline():
    parser = argparse.ArgumentParser(description='Run relaymon integration test (as root)')

    parser.add_argument('-d', '--debug', dest='debug', default=False, action='store_true',
                         help='debug')
    parser.add_argument('-c', '--cleanup', dest='cleanup', default=False, action='store_true',
                         help='cleanup and exit')
    parser.add_argument('-s', '--services', dest='services', default=False, action='store_true',
                         help='create systemd services and exit')

    return parser.parse_args()


def get_exception_loc():
    exc_type, exc_obj, tb = sys.exc_info()
    f = tb.tb_frame
    lineno = tb.tb_lineno
    filename = f.f_code.co_filename
    return (filename, lineno)


def cleanupServices(names):
    global debug
    if debug:
        outredir = ""
    else:
        outredir = ">/dev/null 2>&1"

    found = False
    for name in names:
        fname = "/etc/systemd/system/%s.service" % name
        try:
            code = subprocess.call("systemctl status %s%s" % (name, outredir), shell = True)
            if code == 0:
                found = True
                code = subprocess.call("systemctl stop %s%s" % (name, outredir), shell = True)
                if code != 0:
                    sys.stderr.write("systemctl stop %s exit with %d\n" % (name, code))
            if os.path.exists(fname):
                sys.stdout.write("cleanup systemd: %s\n" % name)
                found = True
                os.unlink(fname)
        except Exception as e:
            sys.stderr.write("%s: %s\n" % (fname, str(e)))

    if found:
        subprocess.call("systemctl daemon-reload", shell = True)


def setServices(names, start):
    global debug
    if debug:
        outredir = ""
    else:
        outredir = " >/dev/null 2>&1"

    n = 0
    for name in names:
        code = subprocess.call("systemctl status %s%s" % (name, outredir), shell = True)
        if code == 0 and not start[n]:
            code = subprocess.call("systemctl stop %s%s" % (name, outredir), shell = True)
            if code != 0:
                raise RuntimeError("systemctl stop %s exit with %d" % (name, code))
            else:
                code = subprocess.call("systemctl status %s%s" % (name, outredir), shell = True)
                if code != 3:
                    raise RuntimeError("systemctl status %s exit with %d after stop" % (name, code))    
                sys.stdout.write("stop systemd: %s\n" % name)
        elif code != 0 and start[n]:
            code = subprocess.call("systemctl start %s%s" % (name, outredir), shell = True)
            if code != 0:
                raise RuntimeError("systemctl start %s exit with %d" % (name, code))
            else:
                code = subprocess.call("systemctl status %s%s" % (name, outredir), shell = True)
                if code != 0:
                    raise RuntimeError("systemctl status %s exit with %d after start" % (name, code))
                sys.stdout.write("start systemd: %s\n" % name)

        n += 1


def createServices(names, template):
    global debug
    if debug:
        outredir = ""
    else:
        outredir = ">/dev/null 2>&1"

    try:
        for name in names:
            proc = subprocess.Popen("systemctl status %s" % name, shell = True, stdout=subprocess.PIPE)
            outs, errs = proc.communicate()
            code = proc.wait()
            if debug:
                sys.stdout.write(outs)
            if code != 4:
                # Workaround for not found
                if code == 3:
                    if 'Loaded: not-found (' in str(outs):
                        continue
                sys.stderr.write("systemctl status %s exit with %d\n" % (name, code))
                return False
    except Exception as e:
        sys.stderr.write("%s\n" % str(e))
        return False
    
    with open(template, 'r') as ifile:
        data = ifile.read()
        for name in names:
            try:
                with open("/etc/systemd/system/%s.service" % name, 'w') as ofile:
                    replaced = Template(data).substitute(dict({'name': name}))
                    ofile.write(replaced)
                    sys.stdout.write("create systemd: %s\n" % name)
                    subprocess.call("systemctl status %s%s" % (name, outredir), shell = True)
            except Exception as e:
                raise RuntimeError("can't create systemd service %s: %s" % (name, str(e)))
  
    code = subprocess.call("systemctl daemon-reload", shell = True)
    if code != 0:
        raise RuntimeError("can't create reload systemd")

    return True


def cmdIPDel(iface, ip, netmask, scope):
    if scope is None or scope == "":
        return "ip addr del dev %s %s/%s" % (iface, ip, IPAddress(netmask).netmask_bits())
    else:    
        return "ip addr del dev %s %s/%s scope %s" % (iface, ip, IPAddress(netmask).netmask_bits(), scope)

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
            sys.stderr.write("configured %s/%s on %s\n" % (ips[i][0], ips[i][1], iface))
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


class Server(object):
    def __init__(self, hostname, port):
        self.logger = logging.getLogger("Server")
        self.hostname = hostname
        self.port = port
        self.ln = None
        self.started = False
        self.childs = []

    def bind(self):
        self.ln = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.ln.bind((self.hostname, self.port))
        self.port = self.ln.getsockname()[1]
        self.logger.debug("bind to %s:%d", self.hostname, self.port)

    def listen(self):
        self.ln.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self.ln.listen(10)
        self.logger.debug("listen on %s:%d", self.hostname, self.port)

    def stop(self):
        if self.ln is not None:
            self.logger.debug("shut down on %s:%d", self.hostname, self.port)
            self.ln.close()
            time.sleep(1)
        self.started = False
        for process in multiprocessing.active_children():
            self.logger.debug("shutting down process %r", process)
            process.terminate()
            process.join()

    def start(self):
        process = multiprocessing.Process(target=handleAccept, args=[self.ln])
        process.daemon = False
        process.start()
        self.logger.debug("Started process %r for incoming connection", process)
        self.started = True


def handleAccept(socket):
    logger = logging.getLogger("accept")
    try:
        while True:
            conn, address = socket.accept()
            process = multiprocessing.Process(target=handleConnection, args=(conn, address))
            process.daemon = False
            process.start()
            logger.debug("Started process %r for incoming connection", process)
    finally:
        for process in multiprocessing.active_children():
            logger.debug("shutting down process %r", process)
            process.terminate()
            process.join()


def handleConnection(conn, address):
    logger = logging.getLogger("child-%r" % (address,))
    try:
        logger.debug("Connected from %r", address)
        conn.setblocking(0)
        while True:
            ready = select.select([conn], [], [], 10)
            if ready[0]:
                data = conn.recv(1024)
                if data == "":
                    logger.debug("socket closed remotely")
                    break
            else:
                logger.debug("socket read timeout")
                break
            logger.debug("received data %r", data)
    except:
        logger.exception("problem handling request")
    finally:
        logger.debug("closing socket")
        conn.close()


# Base test function (with 5 tcp servers and 2 systemd services for test)
def test(testName, name, config, services, iface, ips, output, stageAction, stageResult, serversListen, servicesStarted):
    success = False

    configRelayTpl = "carbon-c-relay-test.tpl"
    configRelay = "/tmp/relaymon-carbon-c-relay-test.conf"

    logger = logging.getLogger(testName)

    servers = [
        Server("127.0.0.1", 0),
        Server("127.0.0.1", 0),
        Server("127.0.0.1", 0),
        Server("127.0.0.1", 0),
        Server("127.0.0.1", 0),
    ]
    templateArgs = dict()

    logger.info("start test")
    try:
        try:
            n = 0
            for a in serversListen[0]:
                if a:
                    servers[n].bind()
                    servers[n].listen()
                    servers[n].start()
                    logger.info("start server %d", n)
                else:
                    servers[n].bind()

                templateArgs["port%d" % (n+1)] = servers[n].port
                n += 1
        except Exception as e:
            logger.error("can't start test tcp servers: %s", str(e))
            return False

        setServices(services, servicesStarted[0])

        with open(configRelayTpl, 'r') as ifile:
            data = ifile.read()
            replaced = Template(data).substitute(templateArgs)
            try:
                with open(configRelay, 'w') as ofile:
                    ofile.write(replaced)
            except Exception as e:
                logger.error("can't create carbon-c-relay config: %s", str(e))
                return False

        try:
            proc = subprocess.Popen([name, "-config", config], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            ec = proc.poll()
            if ec is not None:
                logger.error("%s exit with code %s on start", name, ec)
                sys.exit(ec)

            success = True

            stage = 0
            step = 0
            if stageAction[0] is not None:
                logger.error("stageAction not implemented")
                return False

            while stage < len(output):
                if stage > 0:
                    setServices(services, servicesStarted[stage])
                    n = 0
                    for a in serversListen[stage]:
                        if a and not servers[n].started:
                            servers[n].listen()
                            servers[n].start()
                            logger.info("start server %d", n)
                        elif not a and servers[n].started:
                            servers[n].stop()
                            logger.info("stop server %d", n)
                        n += 1
                s = select.select([proc.stdout, proc.stderr], [], [], 60)
                if len(s[0]) == 0:
                    logger.error("read from stdout timeout at step %d", step)
                    success = False
                    break
                if proc.stderr in s[0]:
                    logger.error("stderr non empthy")
                    success = False
                    break
                if proc.stdout in s[0]:
                    line = proc.stdout.readline()
                    sys.stdout.write(line)
                    try:
                        match = re.match(output[stage][step][0], line)
                    except Exception as e:
                        logger.error("stage %d failed, missed step %d output", stage, step)
                        return False
                    if match is not None:
                        groups = match.groups()
                        i = 1
                        for g in groups:
                            if g != output[stage][step][i]:
                                logger.error("step %d:%d stdout mismatched at %d (got %s, want %s): %s", stage, step,
                                    i, g, output[stage][step][i], line.rstrip()
                                )
                                success = False
                                break
                            i += 1
                        if not success:
                            break

                        step += 1
                        logger.info("step %d:%d ended", stage, step)
                        if step == len(output[stage]):
                            # Step ended
                            found, status = checkIPS(iface, ips)
                            if stageResult[stage]:
                                if not validateIPS(iface, ips, status):
                                    logger.error("stage %s failed", stage)
                                    success = False
                                    break
                            elif found > 0:
                                printIPS(iface, ips, status)
                                logger.error("stage %s failed", stage)
                                success = False
                                break

                            logger.info("stage %s ended", stage)
                            stage += 1
                            step = 0

            logger.info("test shutting down")
            proc.terminate()
            ec = proc.wait()
            for line in proc.stdout:
                sys.stdout.write(line)
            for line in proc.stderr:
                sys.stderr.write(line)
            if ec == 0 and success:
                sys.stdout.write("SUCCESS %s\n" % testName)
                return True
            else:
                logger.error("%s exit with code %s", name, ec)
                sys.stdout.write("FAILED %s\n" % testName)
                return False
        except Exception as e:
            (file, line) = get_exception_loc()
            logger.error("%s:%s %s", file, line, str(e))
            return False
    finally:
        if os.path.exists(configRelay):
            os.unlink(configRelay)

        for s in servers:
            s.stop()


def testFailSuccessFail1(name, config, services, iface, ips):
    testName = "test Fail Success Fail #1"

    output = []
    stageAction = []
    stageResult = []
    serversListen = []
    servicesStarted = []

    # Step 0 (startup). Go to failed state
    serversListen.append([False, False, False, False, False])
    servicesStarted.append([True, False])
    stageAction.append(None)
    stageResult.append(False)
    output.append([
        (
            '{"level":"([a-z]+)","service":"%s","time":"[0-9-T:Z\+-]+","message":"state changed to ([a-z]+)"}' % services[0],
            "info", "success", None
        ),
        (
            '{"level":"([a-z]+)","service":"%s","time":"[0-9-T:Z\+-]+","message":"state changed to ([a-z]+)"}' % services[1],
            "error", "error", None
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
            "info", "down", "DOWN"
        ),
    ])

    # Step 1 (Up one endpoint). Go to success state
    serversListen.append([False, False, False, False, True])
    servicesStarted.append([True, True])
    stageAction.append(None)
    stageResult.append(True)
    output.append([
        (
            '{"level":"([a-z]+)","service":"%s","time":"[0-9-T:Z\+-]+","message":"state changed to ([a-z]+)"}' % services[1],
            "info", "success", None
        ),
        (
            '{"level":"([a-z]+)","service":"carbon-c-relay clusters","time":"[0-9-T:Z\+-]+","message":"state changed to ([a-z]+)"}',
            "info", "success", None
        ),
        (
            '{"level":"([a-z]+)","action":"([a-z]+)","time":"[0-9-T:Z\+-]+","message":"([a-zA-Z ]+)"}',
            "info", "up", "IP addresses configured"
        ),
        (
            '{"level":"([a-z]+)","action":"([a-z]+)","time":"[0-9-T:Z\+-]+","message":"([A-Z]+)"}',
            "info", "up", "UP"
        ),
    ])

    # Step 2 (Down all endpoint). Go to warning state
    serversListen.append([False, False, False, False, False])
    servicesStarted.append([True, True])
    stageAction.append(None)
    stageResult.append(True)
    output.append([
        (
            '{"level":"([a-z]+)","service":"carbon-c-relay clusters","time":"[0-9-T:Z\+-]+","message":"state changed to ([a-z]+)"}',
            "warn", "warning", None
        ),
    ])

    # Step 3. Go to failed state
    serversListen.append([False, False, False, False, False])
    servicesStarted.append([True, True])
    stageAction.append(None)
    stageResult.append(False)
    output.append([
        (
            '{"level":"([a-z]+)","service":"carbon-c-relay clusters","time":"[0-9-T:Z\+-]+","message":"state changed to ([a-z]+)"}',
            "error", "error", None
        ),
        (
            '{"level":"([a-z]+)","action":"([a-z]+)","time":"[0-9-T:Z\+-]+","message":"go to ([a-z]+) state"}',
            "error", "down", "error"
        ),
        (
            '{"level":"([a-z]+)","action":"([a-z]+)","time":"[0-9-T:Z\+-]+","message":"([a-zA-Z ]+)"}',
            "info", "down", "IP addresses deconfigured"
        ),
        (
            '{"level":"([a-z]+)","action":"([a-z]+)","time":"[0-9-T:Z\+-]+","message":"([A-Z]+)"}',
            "info", "down", "DOWN"
        ),
    ])

    # Step 4. Go to success state
    serversListen.append([True, True, True, True])
    servicesStarted.append([True, True])
    stageAction.append(None)
    stageResult.append(True)
    output.append([
        (
            '{"level":"([a-z]+)","service":"carbon-c-relay clusters","time":"[0-9-T:Z\+-]+","message":"state changed to ([a-z]+)"}',
            "info", "success", None
        ),
        (
            '{"level":"([a-z]+)","action":"([a-z]+)","time":"[0-9-T:Z\+-]+","message":"([a-zA-Z ]+)"}',
            "info", "up", "IP addresses configured"
        ),
        (
            '{"level":"([a-z]+)","action":"([a-z]+)","time":"[0-9-T:Z\+-]+","message":"([A-Z]+)"}',
            "info", "up", "UP"
        ),
    ])

    # Step 5 (Shutdown relaymontest1). Go to warning state
    serversListen.append([True, True, True, True])
    servicesStarted.append([False, True])
    stageAction.append(None)
    stageResult.append(True)
    output.append([
        (
            '{"level":"([a-z]+)","service":"%s","time":"[0-9-T:Z\+-]+","message":"state changed to ([a-z]+)"}' % services[0],
            "warn", "warning", None
        ),
    ])

    # Step 6. Go to failed state
    serversListen.append([True, True, True, True])
    servicesStarted.append([False, True])
    stageAction.append(None)
    stageResult.append(False)
    output.append([
        (
            '{"level":"([a-z]+)","service":"%s","time":"[0-9-T:Z\+-]+","message":"state changed to ([a-z]+)"}' % services[0],
            "error", "error", None
        ),
        (
            '{"level":"([a-z]+)","action":"([a-z]+)","time":"[0-9-T:Z\+-]+","message":"go to ([a-z]+) state"}',
            "error", "down", "error"
        ),
        (
            '{"level":"([a-z]+)","action":"([a-z]+)","time":"[0-9-T:Z\+-]+","message":"([a-zA-Z ]+)"}',
            "info", "down", "IP addresses deconfigured"
        ),
        (
            '{"level":"([a-z]+)","action":"([a-z]+)","time":"[0-9-T:Z\+-]+","message":"([A-Z]+)"}',
            "info", "down", "DOWN"
        ),
    ])

    # (
    #     '{"level":"([a-z]+)","time":"2020-08-26T10:49:59Z","message":"shutdown"}', 
    # )

    return test(testName, name, config, services, iface, ips, output, stageAction, stageResult, serversListen, servicesStarted)

def main():
    global debug
    args = parse_cmdline()
    debug = args.debug
    if debug:
        logging.basicConfig(level=logging.DEBUG)
    else:
        logging.basicConfig(level=logging.INFO)

    if os.getuid() != 0:
        sys.stderr.write("run as root for network configuration testing\n")
        sys.exit(255)

    name = "./relaymon"
    config = "relaymon-test.yml"

    services = ["relaymontest1", "relaymontest2"]
    systemdTemplate = "systemd.service"

    iface = "lo"
    scope = None
    ips = [("192.168.155.10", "255.255.255.0"), ("192.168.155.11", "255.255.255.0")]

    success = True

    if args.services:
        createServices(services, systemdTemplate)
        sys.exit(0)

    if args.cleanup:
        found, status = checkIPS(iface, ips)
        if found > 0:
            printIPS(iface, ips, status)
            #sys.stderr.write("For remove:\nip addr del dev lo IP/NETMASK scope %s\n" % scope)
            sys.stderr.write("For remove:\n")
            n = 0
            for ip in ips:
                cmd = ip
                try:
                    if status[n][1]:
                        cmd = cmdIPDel(iface, ip[0], ip[1], scope)
                        subprocess.call(cmd, shell = True)
                        sys.stdout.write("cleanup %s\n", ip[0])
                except Exception as e:
                    sys.stderr.write("cleanup %s: %s\n", cmd, str(e))
                n += 1

        cleanupServices(services)
        sys.exit(1)

    found, status = checkIPS(iface, ips)
    if found > 0:
        printIPS(iface, ips, status)
        #sys.stderr.write("For remove:\nip addr del dev lo IP/NETMASK scope %s\n" % scope)
        sys.stderr.write("For remove:\n")
        n = 0
        for ip in ips:
            if status[n][1]:
                sys.stderr.write("%s\n" % cmdIPDel(iface, ip[0], status[n][1], scope))
            n += 1

        sys.exit(255)

    try:
        if not createServices(services, systemdTemplate):
            sys.exit(255)
    except Exception as e:
        sys.stderr.write("%s\n" % str(e))
        cleanupServices(services)

    interrupted = False
    try:
        if not testFailSuccessFail1(name, config, services, iface, ips):
            success = False
    except KeyboardInterrupt:
        interrupted = True
    finally:
        logger = logging.getLogger("cleanup")

    found, status = checkIPS(iface, ips)
    if found > 0:
        printIPS(iface, ips, status)
        #sys.stderr.write("For remove:\nip addr del dev lo IP/NETMASK scope %s\n" % scope)
        sys.stderr.write("For remove:\n")
        n = 0
        for ip in ips:
            cmd = ip
            try:
                if status[n][1]:
                    cmd = cmdIPDel(iface, ip[0], ip[1], scope)
                    subprocess.call(cmd, shell = True)
                    logger.info("cleanup %s", ip[0])
            except Exception as e:
                logger.error("cleanup %s: %s", cmd, str(e))
            n += 1

    cleanupServices(services)

    if not interrupted:
        if success:
            sys.stdout.write("SUCCESS\n")
            sys.exit(0)
        else:
            sys.stdout.write("FAILED\n")
            sys.exit(1)
    
    
if __name__ == "__main__":
    main()
