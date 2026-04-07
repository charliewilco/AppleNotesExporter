use framework "Foundation"
use scripting additions

property NSMutableArray : a reference to current application's NSMutableArray
property NSMutableDictionary : a reference to current application's NSMutableDictionary
property NSJSONSerialization : a reference to current application's NSJSONSerialization
property NSDateFormatter : a reference to current application's NSDateFormatter
property NSLocale : a reference to current application's NSLocale
property NSTimeZone : a reference to current application's NSTimeZone
property NSString : a reference to current application's NSString
property NSUTF8StringEncoding : a reference to current application's NSUTF8StringEncoding

on run argv
	set formatter to NSDateFormatter's alloc()'s init()
	formatter's setLocale:(NSLocale's localeWithLocaleIdentifier:"en_US_POSIX")
	formatter's setTimeZone:(NSTimeZone's timeZoneWithAbbreviation:"UTC")
	formatter's setDateFormat:"yyyy-MM-dd'T'HH:mm:ss'Z'"

	set exportedNotes to NSMutableArray's alloc()'s init()

	tell application "Notes"
		repeat with currentNote in every note
			set notePayload to my payloadForNote(currentNote, formatter)
			(exportedNotes's addObject:notePayload)
		end repeat
	end tell

	set {jsonData, serializationError} to NSJSONSerialization's dataWithJSONObject:exportedNotes options:0 |error|:(reference)
	if jsonData is missing value then
		error ((serializationError's localizedDescription()) as text)
	end if

	return (NSString's alloc()'s initWithData:jsonData encoding:NSUTF8StringEncoding) as text
end run

on payloadForNote(currentNote, formatter)
	set notePayload to NSMutableDictionary's alloc()'s init()
	set noteID to ""
	set noteTitle to ""

	tell application "Notes"
		try
			set noteID to (id of currentNote) as text
		end try

		try
			set noteTitle to (name of currentNote) as text
		end try

		notePayload's setObject:(my safeText(noteID)) forKey:"id"
		notePayload's setObject:(my safeText(noteTitle)) forKey:"title"

		try
			notePayload's setObject:((body of currentNote) as text) forKey:"body"
		on error errorMessage number errorNumber
			notePayload's setObject:((errorNumber as text) & ": " & errorMessage) forKey:"fetch_error"
			return notePayload
		end try

		set folderInfo to my folderInfoForContainer(container of currentNote)
		notePayload's setObject:(item 1 of folderInfo) forKey:"folder"
		notePayload's setObject:(item 2 of folderInfo) forKey:"folder_path"
		notePayload's setObject:(item 3 of folderInfo) forKey:"account"
		notePayload's setObject:(my formatDate(creation date of currentNote, formatter)) forKey:"created"
		notePayload's setObject:(my formatDate(modification date of currentNote, formatter)) forKey:"modified"
	end tell

	return notePayload
end payloadForNote

on folderInfoForContainer(folderRef)
	tell application "Notes"
		set folderName to my safeText(name of folderRef)
		set parentContainer to container of folderRef

		if class of parentContainer is account then
			set accountName to my safeText(name of parentContainer)
			return {folderName, folderName, accountName}
		end if

		if class of parentContainer is folder then
			set parentInfo to my folderInfoForContainer(parentContainer)
			set accountName to item 3 of parentInfo
			set parentPath to item 2 of parentInfo
			if parentPath is "" then
				return {folderName, folderName, accountName}
			end if

			return {folderName, parentPath & "/" & folderName, accountName}
		end if
	end tell

	return {folderName, folderName, ""}
end folderInfoForContainer

on safeText(valueToClean)
	if valueToClean is missing value then
		return ""
	end if

	return valueToClean as text
end safeText

on formatDate(inputDate, formatter)
	if inputDate is missing value then
		return ""
	end if

	return (formatter's stringFromDate:inputDate) as text
end formatDate
