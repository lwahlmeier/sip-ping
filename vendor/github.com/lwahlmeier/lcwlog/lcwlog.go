package lcwlog

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cornelk/hashmap"
	"github.com/lwahlmeier/pyfmt"
	"github.com/tevino/abool"
)

type Logger interface {
	Debug(...interface{})
	Warn(...interface{})
	Fatal(...interface{})
}

// LCWLogger this struct is generic generic logger used.
type LCWLogger interface {
	Trace(...interface{})
	Debug(...interface{})
	Verbose(...interface{})
	Info(...interface{})
	Warn(...interface{})
	Fatal(...interface{})
	GetLogLevel() Level
}
type LCWLoggerConfig interface {
	SetLogger(Logger)
	SetLevel(Level)
	SetDateFormat(string)
	AddLogFile(string, Level) error
	RemoveLogFile(string)
	ForceFlush(bool)
	Flush()
	EnableLevelLogging(bool)
	EnableTimeLogging(bool)
}

// Level is the Level of logging set
type Level int32

const (
	defaultLevel Level = -1
	//FatalLevel this is used to log an error that will cause fatal problems in the program
	FatalLevel Level = 0
	//WarnLevel is logging for interesting events that need to be known about but are not crazy
	WarnLevel    Level = 20
	InfoLevel    Level = 30
	VerboseLevel Level = 40
	DebugLevel   Level = 50
	TraceLevel   Level = 60
)

type logFile struct {
	path     string
	logLevel Level
	fp       *os.File
}

type logMessage struct {
	logLevel Level
	ltime    time.Time
	msg      string
	args     []interface{}
	wg       *sync.WaitGroup
}

type fullLCWLogger struct {
	setLogger    Logger
	currentLevel Level
	highestLevel Level
	dateFMT      string
	logfiles     hashmap.HashMap
	logQueue     chan *logMessage
	forceFlush   *abool.AtomicBool
	logLevel     bool
	logTime      bool
}

var logger *fullLCWLogger

var prefixLogger map[string]LCWLogger

const traceMsg = "[ TRACE ]"
const debugMsg = "[ DEBUG ]"
const warnMsg = "[ WARN  ]"
const fatalMsg = "[ FATAL ]"
const infoMsg = "[ INFO  ]"
const verboseMsg = "[VERBOSE]"
const dateFMT = "2006-01-02 15:04:05.999"

var LCWLoggerCreateLock sync.Mutex = sync.Mutex{}

func resetLogger() {
	logger = nil
	prefixLogger = nil
}

func GetLoggerConfig() LCWLoggerConfig {
	GetLogger()
	return logger
}

//GetLogger gets a logger for logging.
func GetLogger() LCWLogger {
	if logger == nil {
		LCWLoggerCreateLock.Lock()
		defer LCWLoggerCreateLock.Unlock()
		if logger == nil {
			logger = &fullLCWLogger{
				currentLevel: InfoLevel,
				highestLevel: InfoLevel,
				dateFMT:      dateFMT,
				logQueue:     make(chan *logMessage, 1000),
				logfiles:     hashmap.HashMap{},
				forceFlush:   abool.NewBool(true),
				logLevel:     true,
				logTime:      true,
			}
			logger.AddLogFile("STDOUT", defaultLevel)
			go logger.writeLogQueue()
		}
	}
	return logger
}

//GetLoggerWithPrefix gets a logger for logging with a static prefix.
func GetLoggerWithPrefix(prefix string) LCWLogger {
	baseLogger := GetLogger()
	if prefix == "" {
		return baseLogger
	}
	if prefixLogger == nil {
		LCWLoggerCreateLock.Lock()
		if prefixLogger == nil {
			prefixLogger = make(map[string]LCWLogger)
		}
		LCWLoggerCreateLock.Unlock()
	}
	LCWLoggerCreateLock.Lock()
	defer LCWLoggerCreateLock.Unlock()
	if sl, ok := prefixLogger[prefix]; ok {
		return sl
	}
	prefixLogger[prefix] = &lcwPrefixLogger{LCWLogger: logger, prefix: prefix}
	return prefixLogger[prefix]
}

//GetLogLevel gets the current highest set log level for lcwlog
func (LCWLogger *fullLCWLogger) GetLogLevel() Level {
	return LCWLogger.highestLevel
}

//EnableLevelLogging enables/disables logging of the level (WARN/DEBUG, etc)
func (LCWLogger *fullLCWLogger) EnableLevelLogging(b bool) {
	LCWLogger.logLevel = b
}

//EnableTimeLogging enables/disables logging of the timestamp
func (LCWLogger *fullLCWLogger) EnableTimeLogging(b bool) {
	LCWLogger.logTime = b
}

//RemoveLogFile removes logging of a file (can be STDOUT/STDERR too)
func (LCWLogger *fullLCWLogger) RemoveLogFile(file string) {
	_, ok := LCWLogger.logfiles.Get(file)
	if ok {
		highestLL := defaultLevel
		LCWLogger.logfiles.Del(file)
		for kv := range LCWLogger.logfiles.Iter() {
			lgr := kv.Value.(*logFile)
			if lgr.logLevel > highestLL {
				highestLL = lgr.logLevel
			}
		}
		if highestLL > defaultLevel {
			LCWLogger.highestLevel = highestLL
		}
	}
}

//AddLogFile adds logging of a file (can be STDOUT/STDERR too)
func (LCWLogger *fullLCWLogger) AddLogFile(file string, logLevel Level) error {
	var fp *os.File
	var err error
	if file == "STDOUT" {
		fp = os.Stdout
	} else if file == "STDERR" {
		fp = os.Stderr
	} else {
		fp, err = os.OpenFile(file, os.O_RDWR|os.O_CREATE, 0750)
		if err != nil {
			return err
		}
		fs, err := fp.Stat()
		if err != nil {
			return err
		}
		fp.Seek(fs.Size(), 0)
	}
	if logLevel > LCWLogger.highestLevel {
		LCWLogger.highestLevel = logLevel
	}
	LCWLogger.logfiles.Set(file, &logFile{path: file, logLevel: logLevel, fp: fp})
	return nil
}

func (LCWLogger *fullLCWLogger) writeLogQueue() {
	syncSoon := time.Duration(100 * time.Millisecond)
	syncLate := time.Duration(10000 * time.Millisecond)
	delay := time.NewTimer(syncSoon)
	if LCWLogger.forceFlush.IsSet() {
		delay.Reset(syncLate)
	}
	defer delay.Stop()
	for {
		if LCWLogger.forceFlush.IsSet() {
			delay.Reset(syncLate)
		} else {
			delay.Reset(syncSoon)
		}
		select {
		case lm := <-LCWLogger.logQueue:
			LCWLogger.writeLogs(lm.logLevel, lm.wg != nil, LCWLogger.formatLogMessage(lm))
			if lm.wg != nil {
				lm.wg.Done()
			}
		case <-delay.C:
			for kv := range LCWLogger.logfiles.Iter() {
				lgr := kv.Value.(*logFile)
				lgr.fp.Sync()
			}
		}
	}
}

func (LCWLogger *fullLCWLogger) formatLogMessage(lm *logMessage) string {
	var sb strings.Builder
	if LCWLogger.logTime {
		sb.WriteString(lm.ltime.Format(dateFMT))
		sb.WriteString("\t")
	}
	if LCWLogger.logLevel {
		if lm.logLevel == FatalLevel {
			sb.WriteString(fatalMsg)
		} else if lm.logLevel == WarnLevel {
			sb.WriteString(warnMsg)
		} else if lm.logLevel == InfoLevel {
			sb.WriteString(infoMsg)
		} else if lm.logLevel == VerboseLevel {
			sb.WriteString(verboseMsg)
		} else if lm.logLevel == DebugLevel {
			sb.WriteString(debugMsg)
		} else if lm.logLevel == TraceLevel {
			sb.WriteString(traceMsg)
		} else {
			sb.WriteString(strconv.FormatInt(int64(lm.logLevel), 10))
		}
		sb.WriteString("\t")
	}
	sb.WriteString(pyfmt.Must(lm.msg, lm.args...))
	// sb.WriteString(argFormatter1(lm.msg, lm.args...))
	sb.WriteString("\n")
	return sb.String()
}

func (LCWLogger *fullLCWLogger) writeLogs(logLevel Level, sync bool, msg string) {
	for kv := range LCWLogger.logfiles.Iter() {
		lgr := kv.Value.(*logFile)
		if lgr.logLevel >= logLevel || (lgr.logLevel == defaultLevel && LCWLogger.currentLevel >= logLevel) {
			lgr.fp.WriteString(msg)
			if LCWLogger.forceFlush.IsSet() || sync {
				lgr.fp.Sync()
			}
		}
	}
}

func (LCWLogger *fullLCWLogger) wrapMessage(ll Level, wg *sync.WaitGroup, args ...interface{}) *logMessage {
	st := time.Now()
	var msg string
	switch args[0].(type) {
	case string:
		msg = args[0].(string)
	default:
		msg = fmt.Sprintf("%v", args[0])
	}
	return &logMessage{
		ltime:    st,
		msg:      msg,
		args:     args[1:],
		logLevel: ll,
		wg:       wg,
	}
}

//Forces logger to write all current logs, will block till done
func (LCWLogger *fullLCWLogger) Flush() {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	LCWLogger.logQueue <- LCWLogger.wrapMessage(TraceLevel, wg, "Flush")
	wg.Wait()
}

//ForceFlush enables/disables forcing sync on logfiles after every write
func (LCWLogger *fullLCWLogger) ForceFlush(ff bool) {
	LCWLogger.forceFlush.SetTo(ff)
	if ff {
		LCWLogger.Flush()
	}
}

//SetDateFormat allows you to set how the date/time is formated
func (LCWLogger *fullLCWLogger) SetDateFormat(nf string) {
	LCWLogger.dateFMT = nf
}

// SetLogger takes a structured logger to interface with.
// After the logger is setup it will be available across your packages
// If SetLogger is not used Debug will not create output
func (LCWLogger *fullLCWLogger) SetLogger(givenLogger Logger) {
	LCWLogger.setLogger = givenLogger
}

// SetLevel sets the LCWLogger log level.
func (LCWLogger *fullLCWLogger) SetLevel(level Level) {
	LCWLogger.currentLevel = level
	hl := level
	for kv := range LCWLogger.logfiles.Iter() {
		lgr := kv.Value.(*logFile)
		if lgr.logLevel > hl {
			hl = lgr.logLevel
		}
	}
	LCWLogger.highestLevel = hl
}

// Debug logs a message at level Debug on the standard logger.
func (LCWLogger *fullLCWLogger) Debug(message ...interface{}) {
	if LCWLogger.highestLevel >= DebugLevel {
		if LCWLogger.setLogger == nil {
			if LCWLogger.forceFlush.IsSet() {
				wg := &sync.WaitGroup{}
				wg.Add(1)
				LCWLogger.logQueue <- LCWLogger.wrapMessage(DebugLevel, wg, message...)
				wg.Wait()
			} else {
				LCWLogger.logQueue <- LCWLogger.wrapMessage(DebugLevel, nil, message...)
			}
		} else {
			LCWLogger.setLogger.Debug(message...)
		}
	}
}

//Verbose logs a message at level Verbose on the standard logger.
func (LCWLogger *fullLCWLogger) Verbose(message ...interface{}) {
	if LCWLogger.highestLevel >= VerboseLevel {
		if LCWLogger.setLogger == nil {
			if LCWLogger.forceFlush.IsSet() {
				wg := &sync.WaitGroup{}
				wg.Add(1)
				LCWLogger.logQueue <- LCWLogger.wrapMessage(VerboseLevel, wg, message...)
				wg.Wait()
			} else {
				LCWLogger.logQueue <- LCWLogger.wrapMessage(VerboseLevel, nil, message...)
			}
		} else {
			LCWLogger.setLogger.Debug(message...)
		}
	}
}

// Warn logs a message at level Warn on the standard logger.
func (LCWLogger *fullLCWLogger) Warn(message ...interface{}) {
	if LCWLogger.highestLevel >= WarnLevel {
		if LCWLogger.setLogger == nil {
			if LCWLogger.forceFlush.IsSet() {
				wg := &sync.WaitGroup{}
				wg.Add(1)
				LCWLogger.logQueue <- LCWLogger.wrapMessage(WarnLevel, wg, message...)
				wg.Wait()
			} else {
				LCWLogger.logQueue <- LCWLogger.wrapMessage(WarnLevel, nil, message...)
			}
		} else {
			LCWLogger.setLogger.Warn(message...)
		}
	}
}

// Trace logs a message at level Warn on the standard logger.
func (LCWLogger *fullLCWLogger) Trace(message ...interface{}) {
	if LCWLogger.highestLevel >= TraceLevel {
		if LCWLogger.setLogger == nil {
			if LCWLogger.forceFlush.IsSet() {
				wg := &sync.WaitGroup{}
				wg.Add(1)
				LCWLogger.logQueue <- LCWLogger.wrapMessage(TraceLevel, wg, message...)
				wg.Wait()
			} else {
				LCWLogger.logQueue <- LCWLogger.wrapMessage(TraceLevel, nil, message...)
			}
		} else {
			LCWLogger.setLogger.Debug(message...)
		}
	}
}

// Info logs a message at level Info on the standard logger.
func (LCWLogger *fullLCWLogger) Info(message ...interface{}) {
	if LCWLogger.highestLevel >= InfoLevel {
		if LCWLogger.setLogger == nil {

			if LCWLogger.forceFlush.IsSet() {
				wg := &sync.WaitGroup{}
				wg.Add(1)
				LCWLogger.logQueue <- LCWLogger.wrapMessage(InfoLevel, wg, message...)
				wg.Wait()
			} else {
				LCWLogger.logQueue <- LCWLogger.wrapMessage(InfoLevel, nil, message...)
			}

		} else {
			LCWLogger.setLogger.Debug(message...)
		}
	}
}

// Fatal logs a message at level Fatal on the standard logger then the process will exit with status set to 1.
func (LCWLogger *fullLCWLogger) Fatal(message ...interface{}) {
	if LCWLogger.highestLevel >= FatalLevel {
		if LCWLogger.setLogger == nil {
			wg := &sync.WaitGroup{}
			wg.Add(1)
			LCWLogger.logQueue <- LCWLogger.wrapMessage(FatalLevel, wg, message...)
			wg.Wait()
		} else {
			LCWLogger.setLogger.Fatal(message...)
		}
		os.Exit(5)
	}
}

type lcwPrefixLogger struct {
	LCWLogger *fullLCWLogger
	prefix    string
}

func (spl *lcwPrefixLogger) prefixLog(i ...interface{}) []interface{} {
	s := fmt.Sprintf("%v", i[0])
	var sb strings.Builder
	sb.WriteString(spl.prefix)
	sb.WriteString(":")
	sb.WriteString(s)
	i[0] = sb.String()
	return i
}
func (spl *lcwPrefixLogger) Trace(i ...interface{}) {
	if spl.LCWLogger.GetLogLevel() >= TraceLevel {
		spl.LCWLogger.Trace(spl.prefixLog(i...)...)
	}
}
func (spl *lcwPrefixLogger) Debug(i ...interface{}) {
	if spl.LCWLogger.GetLogLevel() >= DebugLevel {
		spl.LCWLogger.Debug(spl.prefixLog(i...)...)
	}
}
func (spl *lcwPrefixLogger) Verbose(i ...interface{}) {
	if spl.LCWLogger.GetLogLevel() >= VerboseLevel {
		spl.LCWLogger.Verbose(spl.prefixLog(i...)...)
	}
}
func (spl *lcwPrefixLogger) Info(i ...interface{}) {
	if spl.LCWLogger.GetLogLevel() >= InfoLevel {
		spl.LCWLogger.Info(spl.prefixLog(i...)...)
	}
}
func (spl *lcwPrefixLogger) Warn(i ...interface{}) {
	if spl.LCWLogger.GetLogLevel() >= WarnLevel {
		spl.LCWLogger.Warn(spl.prefixLog(i...)...)
	}
}
func (spl *lcwPrefixLogger) Fatal(i ...interface{}) {
	if spl.LCWLogger.GetLogLevel() >= FatalLevel {
		spl.LCWLogger.Fatal(spl.prefixLog(i...)...)
	}
}
func (spl *lcwPrefixLogger) GetLogLevel() Level {
	return spl.LCWLogger.GetLogLevel()
}
